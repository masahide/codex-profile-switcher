// Package codex runs the installed Codex CLI in the current Codex
// environment.
package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/masahide/codex-profile-switcher/internal/profile"
)

// Runner is a thin child-process wrapper. It does not inspect or modify any
// Codex credential files or CODEX_HOME.
type Runner struct {
	// Binary overrides CX_CODEX_BIN and PATH lookup when non-empty. It is
	// primarily useful for embedding and tests.
	Binary string

	// CommandPrefix is prepended to every command argument. It is primarily
	// useful when Binary is a test helper executable.
	CommandPrefix []string

	// Stdin, Stdout, and Stderr override the streams passed to Codex. Nil
	// values use the corresponding process standard stream.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Environ overrides the environment used to construct a child process.
	// It is primarily useful for tests and embedding.
	Environ func() []string
}

var (
	ErrAPIKeyEmpty           = errors.New("API key is empty")
	blockedCredentialEnvKeys = [...]string{
		"OPENAI_ADMIN_KEY",
		"OPENAI_API_KEY",
		"CX_OPENAI_API_KEY",
		"CODEX_API_KEY",
		"CODEX_ACCESS_TOKEN",
	}
)

// Run launches Codex with args passed in their original order.
func (r *Runner) Run(ctx context.Context, args []string) error {
	cmd, err := r.command(ctx, args)
	if err != nil {
		return err
	}
	return cmd.Run()
}

// CurrentMode asks Codex for its active authentication mode. Status output
// is captured so cx can expose only the normalized mode and never leak other
// status text.
func (r *Runner) CurrentMode(ctx context.Context) (profile.AuthMode, error) {
	cmd, err := r.command(ctx, []string{"login", "status"})
	if err != nil {
		return profile.Unknown, err
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	mode, ok := parseAuthModeOutput(stdout.String() + "\n" + stderr.String())
	if ok && mode == profile.None {
		return profile.None, nil
	}
	if err != nil {
		return profile.Unknown, fmt.Errorf("codex login status failed: %w", err)
	}
	if !ok {
		return profile.Unknown, nil
	}
	return mode, nil
}

// LoginChatGPT delegates ChatGPT account authentication to Codex.
func (r *Runner) LoginChatGPT(ctx context.Context) error {
	return r.runAttached(ctx, []string{"login"})
}

// LoginAPI delegates API-key authentication to Codex. The key is supplied
// only through stdin and is never added to arguments or the child environment.
func (r *Runner) LoginAPI(ctx context.Context, apiKey string) error {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return ErrAPIKeyEmpty
	}

	cmd, err := r.command(ctx, []string{"login", "--with-api-key"})
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(key + "\n")

	// Buffer the non-interactive API login output so a key echoed by a broken
	// child executable cannot reach either output stream, even across write
	// boundaries.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	stdoutErr := writeRedacted(r.stdout(), stdout.String(), key)
	stderrErr := writeRedacted(r.stderr(), stderr.String(), key)
	if runErr != nil {
		return runErr
	}
	if stdoutErr != nil {
		return fmt.Errorf("write Codex output: %w", stdoutErr)
	}
	if stderrErr != nil {
		return fmt.Errorf("write Codex error output: %w", stderrErr)
	}
	return nil
}

// Logout delegates removal of Codex's saved credentials to Codex.
func (r *Runner) Logout(ctx context.Context) error {
	return r.runAttached(ctx, []string{"logout"})
}

func (r *Runner) runAttached(ctx context.Context, args []string) error {
	cmd, err := r.command(ctx, args)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (r *Runner) command(ctx context.Context, args []string) (*exec.Cmd, error) {
	binary, err := r.binary()
	if err != nil {
		return nil, err
	}

	commandArgs := make([]string, 0, len(r.CommandPrefix)+len(args))
	commandArgs = append(commandArgs, r.CommandPrefix...)
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, binary, commandArgs...)
	cmd.Env = sanitizeEnvironment(r.environ())
	cmd.Stdin = r.stdin()
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()
	return cmd, nil
}

func (r *Runner) environ() []string {
	if r.Environ != nil {
		return r.Environ()
	}
	return os.Environ()
}

func (r *Runner) stdin() io.Reader {
	if r.Stdin != nil {
		return r.Stdin
	}
	return os.Stdin
}

func (r *Runner) stdout() io.Writer {
	if r.Stdout != nil {
		return r.Stdout
	}
	return os.Stdout
}

func (r *Runner) stderr() io.Writer {
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

func (r *Runner) binary() (string, error) {
	if r.Binary != "" {
		return r.Binary, nil
	}
	if binary := os.Getenv("CX_CODEX_BIN"); binary != "" {
		return binary, nil
	}
	binary, err := exec.LookPath("codex")
	if err != nil {
		return "", fmt.Errorf("codex executable not found: %w", err)
	}
	return binary, nil
}

func sanitizeEnvironment(environ []string) []string {
	result := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, ok := strings.Cut(entry, "=")
		if ok && isBlockedCredentialEnvKey(key) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func isBlockedCredentialEnvKey(key string) bool {
	for _, blocked := range blockedCredentialEnvKeys {
		if sameEnvKey(key, blocked) {
			return true
		}
	}
	return false
}

func sameEnvKey(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func parseAuthModeOutput(output string) (profile.AuthMode, bool) {
	normalized := strings.ToLower(strings.Join(strings.Fields(output), " "))
	switch {
	case strings.Contains(normalized, "not logged in"),
		strings.Contains(normalized, "not authenticated"),
		strings.Contains(normalized, "no credentials"):
		return profile.None, true
	case strings.Contains(normalized, "logged in") && strings.Contains(normalized, "chatgpt"):
		return profile.ChatGPT, true
	case strings.Contains(normalized, "logged in") && strings.Contains(normalized, "api key"):
		return profile.API, true
	default:
		return profile.Unknown, false
	}
}

func writeRedacted(destination io.Writer, output, secret string) error {
	if secret != "" {
		output = strings.ReplaceAll(output, secret, "[redacted]")
	}
	_, err := io.WriteString(destination, output)
	return err
}

// ExitCode extracts a child process exit code when one is available.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ProcessState != nil {
		if code := exitErr.ProcessState.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}

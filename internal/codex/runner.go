// Package codex runs the installed Codex CLI in an isolated CODEX_HOME.
package codex

import (
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
// Codex credential files.
type Runner struct {
	// Binary overrides CX_CODEX_BIN and PATH lookup when non-empty. It is
	// primarily useful for embedding and tests.
	Binary string

	// ProfilePath optionally overrides profile.Path for tests or embedding.
	ProfilePath func(profile.Profile) (string, error)

	// Stdin overrides the input passed to Codex. A nil value uses os.Stdin.
	// API-key login requires this stream to be non-interactive.
	Stdin io.Reader
}

// ErrAPIKeyRequiresStdin reports that API-key login was attempted with a
// terminal as stdin. Codex's --with-api-key mode reads the key from stdin and
// expects callers to pipe it explicitly.
var ErrAPIKeyRequiresStdin = errors.New("API login requires the API key on stdin")

var blockedCredentialEnvKeys = [...]string{
	"OPENAI_ADMIN_KEY",
	"OPENAI_API_KEY",
	"CODEX_API_KEY",
	"CODEX_ACCESS_TOKEN",
}

// Run launches Codex with args passed in their original order.
func (r *Runner) Run(ctx context.Context, p profile.Profile, args []string) error {
	cmd, err := r.command(ctx, p, args)
	if err != nil {
		return err
	}
	return cmd.Run()
}

// Login delegates authentication to Codex. The API profile uses Codex's
// --with-api-key flow; cx never receives the key as an argument.
func (r *Runner) Login(ctx context.Context, p profile.Profile) error {
	args, err := loginArgs(p)
	if err != nil {
		return err
	}
	if p == profile.API && stdinIsTerminal(r.stdin()) {
		return ErrAPIKeyRequiresStdin
	}
	return r.Run(ctx, p, args)
}

// LoginStatus delegates status inspection to Codex without reading auth.json.
func (r *Runner) LoginStatus(ctx context.Context, p profile.Profile) error {
	return r.Run(ctx, p, []string{"login", "status"})
}

func loginArgs(p profile.Profile) ([]string, error) {
	if !profile.IsValid(p) {
		return nil, profile.ErrInvalidProfile
	}
	args := []string{"login"}
	if p == profile.API {
		args = append(args, "--with-api-key")
	}
	return args, nil
}

func (r *Runner) command(ctx context.Context, p profile.Profile, args []string) (*exec.Cmd, error) {
	if !profile.IsValid(p) {
		return nil, profile.ErrInvalidProfile
	}
	pathFunc := r.ProfilePath
	if pathFunc == nil {
		pathFunc = profile.Path
	}
	profileDir, err := pathFunc(p)
	if err != nil {
		return nil, fmt.Errorf("resolve profile path: %w", err)
	}
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return nil, fmt.Errorf("create profile directory: %w", err)
	}

	binary, err := r.binary()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = withCodexHome(os.Environ(), profileDir)
	cmd.Stdin = r.stdin()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func (r *Runner) stdin() io.Reader {
	if r.Stdin != nil {
		return r.Stdin
	}
	return os.Stdin
}

func stdinIsTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok || file == nil {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
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

func withCodexHome(environ []string, profileDir string) []string {
	result := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		key, _, ok := strings.Cut(entry, "=")
		if ok && (sameEnvKey(key, "CODEX_HOME") || isBlockedCredentialEnvKey(key)) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "CODEX_HOME="+profileDir)
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

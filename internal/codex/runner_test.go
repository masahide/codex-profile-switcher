package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/masahide/codex-profile-switcher/internal/profile"
)

func TestWithCodexHomeReplacesParentValue(t *testing.T) {
	environ := []string{"PATH=/bin", "CODEX_HOME=/parent", "OTHER=value"}
	got := withCodexHome(environ, "/child")
	if strings.Count(strings.Join(got, "\n"), "CODEX_HOME=") != 1 {
		t.Fatalf("CODEX_HOME entries = %v", got)
	}
	if got[len(got)-1] != "CODEX_HOME=/child" {
		t.Fatalf("last environment entry = %q", got[len(got)-1])
	}
	if environ[1] != "CODEX_HOME=/parent" {
		t.Fatalf("input environment was changed: %v", environ)
	}
}

func TestWithCodexHomeRemovesCredentialEnvironment(t *testing.T) {
	environ := []string{
		"OPENAI_ADMIN_KEY=admin-secret",
		"OPENAI_API_KEY=api-secret",
		"CODEX_API_KEY=codex-secret",
		"CODEX_ACCESS_TOKEN=access-secret",
		"KEEP_ME=value",
	}
	got := withCodexHome(environ, "/child")
	joined := strings.Join(got, "\n")
	for _, secret := range []string{"admin-secret", "api-secret", "codex-secret", "access-secret"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("credential value %q was inherited: %v", secret, got)
		}
	}
	for _, key := range []string{"OPENAI_ADMIN_KEY", "OPENAI_API_KEY", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN"} {
		for _, entry := range got {
			entryKey, _, _ := strings.Cut(entry, "=")
			if sameEnvKey(entryKey, key) {
				t.Fatalf("credential environment %q was inherited: %v", key, got)
			}
		}
	}
	if !strings.Contains(joined, "KEEP_ME=value") {
		t.Fatalf("non-credential environment was removed: %v", got)
	}
}

func TestCommandSetsChildHomeAndPassesArguments(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", "/parent")
	runner := &Runner{
		Binary: filepath.Join(root, "fake-codex"),
		ProfilePath: func(p profile.Profile) (string, error) {
			return filepath.Join(root, string(p)), nil
		},
	}
	cmd, err := runner.command(context.Background(), profile.ChatGPT, []string{"exec", "hello world", "--model", "gpt-test"})
	if err != nil {
		t.Fatalf("command returned error: %v", err)
	}
	if got, want := cmd.Args[1:], []string{"exec", "hello world", "--model", "gpt-test"}; !equalStrings(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	values := make(map[string]string)
	for _, value := range cmd.Env {
		key, val, ok := strings.Cut(value, "=")
		if ok {
			values[key] = val
		}
	}
	wantHome := filepath.Join(root, "chatgpt")
	if values["CODEX_HOME"] != wantHome {
		t.Fatalf("child CODEX_HOME = %q, want %q", values["CODEX_HOME"], wantHome)
	}
	if got := os.Getenv("CODEX_HOME"); got != "/parent" {
		t.Fatalf("parent CODEX_HOME = %q, want /parent", got)
	}
	if _, err := os.Stat(wantHome); err != nil {
		t.Fatalf("profile directory was not created: %v", err)
	}
}

func TestLoginArgsAreProfileSpecific(t *testing.T) {
	chatGPTArgs, err := loginArgs(profile.ChatGPT)
	if err != nil {
		t.Fatalf("chatgpt loginArgs returned error: %v", err)
	}
	if !equalStrings(chatGPTArgs, []string{"login"}) {
		t.Fatalf("chatgpt args = %v", chatGPTArgs)
	}
	apiArgs, err := loginArgs(profile.API)
	if err != nil {
		t.Fatalf("api loginArgs returned error: %v", err)
	}
	if !equalStrings(apiArgs, []string{"login", "--with-api-key"}) {
		t.Fatalf("api args = %v", apiArgs)
	}
	if _, err := loginArgs(profile.Profile("work")); err != profile.ErrInvalidProfile {
		t.Fatalf("invalid profile error = %v", err)
	}
}

func TestAPIKeyLoginRejectsTerminalStdin(t *testing.T) {
	terminal, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		t.Skipf("terminal is unavailable: %v", err)
	}
	defer terminal.Close()

	runner := &Runner{Stdin: terminal}
	if err := runner.Login(context.Background(), profile.API); err != ErrAPIKeyRequiresStdin {
		t.Fatalf("Login error = %v, want %v", err, ErrAPIKeyRequiresStdin)
	}
}

func TestAPIKeyLoginAcceptsPipedStdin(t *testing.T) {
	runner := &Runner{Stdin: strings.NewReader("api-key\n")}
	if got := stdinIsTerminal(runner.Stdin); got {
		t.Fatal("pipe reader was detected as a terminal")
	}
}

func TestExitCodeForProcess(t *testing.T) {
	if os.Getenv("CX_RUNNER_EXIT_HELPER") == "1" {
		os.Exit(42)
	}
	t.Setenv("CX_RUNNER_EXIT_HELPER", "1")
	process := exec.Command(os.Args[0], "-test.run=^TestExitCodeForProcess$")
	err := process.Run()
	if got := ExitCode(err); got != 42 {
		t.Fatalf("ExitCode = %d, want 42 (err=%v)", got, err)
	}
}

func TestBinarySelectionPrefersExplicitThenEnvironment(t *testing.T) {
	t.Setenv("CX_CODEX_BIN", "codex-from-environment")
	runner := &Runner{}
	if got, err := runner.binary(); err != nil || got != "codex-from-environment" {
		t.Fatalf("environment binary = %q, %v", got, err)
	}
	runner.Binary = "explicit-codex"
	if got, err := runner.binary(); err != nil || got != "explicit-codex" {
		t.Fatalf("explicit binary = %q, %v", got, err)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

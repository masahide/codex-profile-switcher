package codex

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/masahide/codex-profile-switcher/internal/profile"
)

func TestSanitizeEnvironmentPreservesCodexHomeAndRemovesCredentials(t *testing.T) {
	environ := []string{
		"CODEX_HOME=/parent",
		"OPENAI_ADMIN_KEY=admin-secret",
		"OPENAI_API_KEY=api-secret",
		"CX_OPENAI_API_KEY=cx-secret",
		"CODEX_API_KEY=codex-secret",
		"CODEX_ACCESS_TOKEN=access-secret",
		"KEEP_ME=value",
	}
	got := sanitizeEnvironment(environ)
	joined := strings.Join(got, "\n")
	for _, secret := range []string{"admin-secret", "api-secret", "cx-secret", "codex-secret", "access-secret"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("credential value %q was inherited: %v", secret, got)
		}
	}
	for _, key := range []string{"OPENAI_ADMIN_KEY", "OPENAI_API_KEY", "CX_OPENAI_API_KEY", "CODEX_API_KEY", "CODEX_ACCESS_TOKEN"} {
		for _, entry := range got {
			entryKey, _, _ := strings.Cut(entry, "=")
			if sameEnvKey(entryKey, key) {
				t.Fatalf("credential environment %q was inherited: %v", key, got)
			}
		}
	}
	if !strings.Contains(joined, "CODEX_HOME=/parent") || !strings.Contains(joined, "KEEP_ME=value") {
		t.Fatalf("shared environment was changed: %v", got)
	}
	if environ[0] != "CODEX_HOME=/parent" {
		t.Fatalf("input environment was changed: %v", environ)
	}
}

func TestCommandKeepsParentHomeAndPassesArguments(t *testing.T) {
	runner := &Runner{
		Binary: "codex-test",
		Environ: func() []string {
			return []string{"PATH=/bin", "CODEX_HOME=/parent", "OTHER=value"}
		},
	}
	cmd, err := runner.command(context.Background(), []string{"exec", "hello world", "--model", "gpt-test"})
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
	if values["CODEX_HOME"] != "/parent" {
		t.Fatalf("child CODEX_HOME = %q, want /parent", values["CODEX_HOME"])
	}
	if _, ok := values["CX_OPENAI_API_KEY"]; ok {
		t.Fatal("CX_OPENAI_API_KEY was passed to Codex")
	}
}

func TestParseAuthModeOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   profile.AuthMode
		ok     bool
	}{
		{name: "chatgpt", output: "Logged in using ChatGPT\n", want: profile.ChatGPT, ok: true},
		{name: "api", output: "Logged in using API key\n", want: profile.API, ok: true},
		{name: "none", output: "Not logged in\n", want: profile.None, ok: true},
		{name: "unknown", output: "unexpected status\n", want: profile.Unknown, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseAuthModeOutput(test.output)
			if got != test.want || ok != test.ok {
				t.Fatalf("parseAuthModeOutput(%q) = (%q, %t), want (%q, %t)", test.output, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestCurrentModeUsesCodexStatus(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		exit    int
		want    profile.AuthMode
		wantErr bool
	}{
		{name: "chatgpt", output: "Logged in using ChatGPT", want: profile.ChatGPT},
		{name: "api", output: "Logged in using API key", want: profile.API},
		{name: "none", output: "Not logged in", exit: 1, want: profile.None},
		{name: "unknown", output: "unexpected status", want: profile.Unknown},
		{name: "status failure", output: "unexpected status", exit: 7, want: profile.Unknown, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CX_CODEX_TEST_HELPER", "status")
			t.Setenv("CX_CODEX_TEST_HELPER_OUTPUT", test.output)
			t.Setenv("CX_CODEX_TEST_HELPER_EXIT", strconv.Itoa(test.exit))
			runner := &Runner{
				Binary:        os.Args[0],
				CommandPrefix: []string{"-test.run=^TestCodexRunnerHelper$"},
			}
			got, err := runner.CurrentMode(context.Background())
			if got != test.want {
				t.Fatalf("CurrentMode mode = %q, want %q (err=%v)", got, test.want, err)
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("CurrentMode error = %v, want error: %t", err, test.wantErr)
			}
		})
	}
}

func TestLoginAPIUsesStdinAndRedactsChildOutput(t *testing.T) {
	const apiKey = "dummy-api-key"
	t.Setenv("CX_CODEX_TEST_HELPER", "api-login")
	runner := &Runner{
		Binary:        os.Args[0],
		CommandPrefix: []string{"-test.run=^TestCodexRunnerHelper$"},
		Environ:       os.Environ,
		Stdout:        new(bytes.Buffer),
		Stderr:        new(bytes.Buffer),
	}
	if err := runner.LoginAPI(context.Background(), apiKey); err != nil {
		t.Fatalf("LoginAPI returned error: %v", err)
	}
	stdout := runner.Stdout.(*bytes.Buffer).String()
	stderr := runner.Stderr.(*bytes.Buffer).String()
	for streamName, stream := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if strings.Contains(stream, apiKey) {
			t.Fatalf("API key appeared in %s: %q", streamName, stream)
		}
		if !strings.Contains(stream, "[redacted]") {
			t.Fatalf("%s did not contain redacted child output: %q", streamName, stream)
		}
	}
	if strings.Contains(stdout+stderr, "OPENAI_ADMIN_KEY") {
		t.Fatal("credential environment name appeared in child output")
	}
}

func TestLoginAPIRejectsEmptyKey(t *testing.T) {
	runner := &Runner{Binary: "does-not-run"}
	if err := runner.LoginAPI(context.Background(), " \n"); err != ErrAPIKeyEmpty {
		t.Fatalf("LoginAPI error = %v, want %v", err, ErrAPIKeyEmpty)
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

func TestCodexRunnerHelper(t *testing.T) {
	switch os.Getenv("CX_CODEX_TEST_HELPER") {
	case "status":
		_, _ = io.WriteString(os.Stdout, os.Getenv("CX_CODEX_TEST_HELPER_OUTPUT"))
		_, _ = io.WriteString(os.Stderr, os.Getenv("CX_CODEX_TEST_HELPER_OUTPUT"))
		exitHelper(t)
	case "api-login":
		input, _ := io.ReadAll(os.Stdin)
		_, _ = fmt.Fprintf(os.Stdout, "%s\n%s", strings.Join(os.Args[1:], " "), input)
		_, _ = fmt.Fprint(os.Stderr, string(input))
		return
	}
}

func exitHelper(t *testing.T) {
	exitCode, err := strconv.Atoi(os.Getenv("CX_CODEX_TEST_HELPER_EXIT"))
	if err != nil {
		exitCode = 0
	}
	if exitCode != 0 {
		os.Exit(exitCode)
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

package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRootFromUsesHomeDirectoryByDefault(t *testing.T) {
	root, err := RootFrom(filepath.Join("home", "user"), "")
	if err != nil {
		t.Fatalf("RootFrom returned error: %v", err)
	}
	want := filepath.Join("home", "user", ".codex-profiles")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestRootFromUsesCXHomeOverride(t *testing.T) {
	root, err := RootFrom(filepath.Join("home", "user"), filepath.Join("tmp", "codex-profiles"))
	if err != nil {
		t.Fatalf("RootFrom returned error: %v", err)
	}
	want := filepath.Clean(filepath.Join("tmp", "codex-profiles"))
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestRootUsesCXHomeBeforeUserHome(t *testing.T) {
	t.Setenv("CX_HOME", filepath.Join("tmp", "codex-profiles"))
	root, err := Root()
	if err != nil {
		t.Fatalf("Root returned error: %v", err)
	}
	if root != filepath.Clean(filepath.Join("tmp", "codex-profiles")) {
		t.Fatalf("root = %q", root)
	}
}

func TestPathForProfiles(t *testing.T) {
	root := filepath.Join("home", "user", ".codex-profiles")
	for _, test := range []struct {
		name string
		p    Profile
		want string
	}{
		{name: "chatgpt", p: ChatGPT, want: filepath.Join(root, "chatgpt")},
		{name: "api", p: API, want: filepath.Join(root, "api")},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, err := PathFromRoot(root, test.p)
			if err != nil {
				t.Fatalf("PathFromRoot returned error: %v", err)
			}
			if path != test.want {
				t.Fatalf("path = %q, want %q", path, test.want)
			}
		})
	}
}

func TestInvalidProfile(t *testing.T) {
	if _, err := Parse("work"); err != ErrInvalidProfile {
		t.Fatalf("Parse error = %v, want ErrInvalidProfile", err)
	}
	if _, err := PathFromRoot("root", Profile("work")); err != ErrInvalidProfile {
		t.Fatalf("PathFromRoot error = %v, want ErrInvalidProfile", err)
	}
}

func TestEnsureCreatesProfileDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CX_HOME", root)
	path, err := Ensure(API)
	if err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("permissions = %o, want 700", info.Mode().Perm())
	}
}

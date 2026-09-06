package profile

import "testing"

func TestBuiltinsAreAuthenticationTargets(t *testing.T) {
	got := Builtins()
	want := []AuthMode{ChatGPT, API}
	if len(got) != len(want) {
		t.Fatalf("builtins = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("builtins = %v, want %v", got, want)
		}
	}
}

func TestParseRejectsNonTargetModes(t *testing.T) {
	for _, name := range []string{"plus", "work", "none", "unknown"} {
		if _, err := Parse(name); err != ErrInvalidAuthMode {
			t.Fatalf("Parse(%q) error = %v, want ErrInvalidAuthMode", name, err)
		}
	}
}

func TestKnownModes(t *testing.T) {
	for _, mode := range []AuthMode{ChatGPT, API, None} {
		if !IsKnown(mode) {
			t.Fatalf("IsKnown(%q) = false", mode)
		}
	}
	for _, mode := range []AuthMode{Unknown, "access-token"} {
		if IsKnown(mode) {
			t.Fatalf("IsKnown(%q) = true", mode)
		}
	}
}

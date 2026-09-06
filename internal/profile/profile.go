// Package profile defines the authentication modes supported by cx.
package profile

import "errors"

// AuthMode is the authentication mode reported by Codex.
type AuthMode string

const (
	AuthModeChatGPT AuthMode = "chatgpt"
	AuthModeAPI     AuthMode = "api"
	AuthModeNone    AuthMode = "none"
	AuthModeUnknown AuthMode = "unknown"

	ChatGPT = AuthModeChatGPT
	API     = AuthModeAPI
	None    = AuthModeNone
	Unknown = AuthModeUnknown
)

var ErrInvalidAuthMode = errors.New("invalid authentication mode")

var builtins = [...]AuthMode{ChatGPT, API}

// Builtins returns the target authentication modes supported by cx.
func Builtins() []AuthMode {
	modes := make([]AuthMode, len(builtins))
	copy(modes, builtins[:])
	return modes
}

// Parse validates a target authentication mode without consulting Codex
// state.
func Parse(name string) (AuthMode, error) {
	mode := AuthMode(name)
	if !IsValid(mode) {
		return "", ErrInvalidAuthMode
	}
	return mode, nil
}

// IsValid reports whether mode can be requested as a target mode.
func IsValid(mode AuthMode) bool {
	for _, candidate := range builtins {
		if mode == candidate {
			return true
		}
	}
	return false
}

// IsKnown reports whether mode is a recognized result from Codex.
func IsKnown(mode AuthMode) bool {
	switch mode {
	case ChatGPT, API, None:
		return true
	default:
		return false
	}
}

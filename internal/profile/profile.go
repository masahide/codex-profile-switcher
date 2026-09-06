// Package profile contains the small amount of profile path policy owned by cx.
package profile

import (
	"errors"
	"os"
	"path/filepath"
)

const (
	ChatGPT Profile = "chatgpt"
	API     Profile = "api"
)

const defaultRootName = ".codex-profiles"

// Profile is one of the built-in Codex environments.
type Profile string

var ErrInvalidProfile = errors.New("invalid profile")

var builtins = [...]Profile{ChatGPT, API}

// Builtins returns the profiles supported by the MVP in display order.
func Builtins() []Profile {
	profiles := make([]Profile, len(builtins))
	copy(profiles, builtins[:])
	return profiles
}

// Parse validates a profile name without consulting the filesystem.
func Parse(name string) (Profile, error) {
	profile := Profile(name)
	if !IsValid(profile) {
		return "", ErrInvalidProfile
	}
	return profile, nil
}

// IsValid reports whether p is one of the built-in profiles.
func IsValid(p Profile) bool {
	for _, candidate := range builtins {
		if p == candidate {
			return true
		}
	}
	return false
}

// Root returns the profile root. CX_HOME takes precedence over the user's
// home directory when it is set to a non-empty value.
func Root() (string, error) {
	if cxHome := os.Getenv("CX_HOME"); cxHome != "" {
		return RootFrom("", cxHome)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return RootFrom(home, "")
}

// RootFrom is the testable form of Root.
func RootFrom(homeDir, cxHome string) (string, error) {
	if cxHome != "" {
		return filepath.Clean(cxHome), nil
	}
	if homeDir == "" {
		return "", errors.New("user home directory is empty")
	}
	return filepath.Join(homeDir, defaultRootName), nil
}

// Path returns the CODEX_HOME directory for p.
func Path(p Profile) (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return PathFromRoot(root, p)
}

// PathFromRoot returns the CODEX_HOME directory for p below root.
func PathFromRoot(root string, p Profile) (string, error) {
	if !IsValid(p) {
		return "", ErrInvalidProfile
	}
	if root == "" {
		return "", errors.New("profile root is empty")
	}
	return filepath.Join(root, string(p)), nil
}

// Ensure creates the profile directory with restrictive permissions on Unix.
// Existing permissions are left untouched; Codex owns the files inside it.
func Ensure(p Profile) (string, error) {
	directory, err := Path(p)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	return directory, nil
}

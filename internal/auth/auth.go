// Package auth coordinates authentication-mode changes through Codex's
// official login and logout commands.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/masahide/codex-profile-switcher/internal/profile"
)

const APIKeyEnvironment = "CX_OPENAI_API_KEY"

var (
	ErrUnknownAuthMode = errors.New("unable to determine current Codex authentication mode")
	ErrAPIKeyRequired  = errors.New("CX_OPENAI_API_KEY is required to switch to API authentication")
)

// Manager is the small authentication surface needed by the CLI. Implemented
// operations are delegated to Codex; cx never reads or writes credentials.
type Manager interface {
	CurrentMode(context.Context) (profile.AuthMode, error)
	LoginChatGPT(context.Context) error
	LoginAPI(context.Context, string) error
	Logout(context.Context) error
}

// SwitchError describes an authentication operation that failed. AfterLogout
// tells the caller that Codex is no longer authenticated and must not be
// started.
type SwitchError struct {
	AfterLogout bool
	Err         error
}

func (e *SwitchError) Error() string {
	if e == nil || e.Err == nil {
		return "authentication operation failed"
	}
	if e.AfterLogout {
		return "authentication switch failed after logout: " + e.Err.Error()
	}
	return "authentication operation failed: " + e.Err.Error()
}

func (e *SwitchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsAfterLogout reports whether err left Codex logged out after a failed
// switch. cx deliberately does not attempt to restore the previous
// credential because it never stores or interprets credential data.
func IsAfterLogout(err error) bool {
	var switchErr *SwitchError
	return errors.As(err, &switchErr) && switchErr.AfterLogout
}

// EnsureAuthMode changes the current Codex authentication mode when needed.
// API-key availability is checked before logout so a missing key cannot
// destroy an existing ChatGPT login.
func EnsureAuthMode(ctx context.Context, manager Manager, target profile.AuthMode, apiKey string) error {
	if manager == nil {
		return errors.New("authentication manager is not configured")
	}
	if !profile.IsValid(target) {
		return profile.ErrInvalidAuthMode
	}

	current, err := manager.CurrentMode(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnknownAuthMode, err)
	}
	if !profile.IsKnown(current) || current == profile.Unknown {
		return ErrUnknownAuthMode
	}
	if current == target {
		return nil
	}
	if target == profile.API && strings.TrimSpace(apiKey) == "" {
		return ErrAPIKeyRequired
	}

	loggedOut := false
	if current != profile.None {
		if err := manager.Logout(ctx); err != nil {
			return &SwitchError{Err: err}
		}
		loggedOut = true
	}

	var loginErr error
	if target == profile.API {
		loginErr = manager.LoginAPI(ctx, apiKey)
	} else {
		loginErr = manager.LoginChatGPT(ctx)
	}
	if loginErr != nil {
		return &SwitchError{AfterLogout: loggedOut, Err: loginErr}
	}
	return nil
}

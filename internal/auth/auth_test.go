package auth

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/masahide/codex-profile-switcher/internal/profile"
)

type fakeManager struct {
	current    profile.AuthMode
	currentErr error
	events     []string
	apiKey     string
	logoutErr  error
	loginErr   error
}

func (f *fakeManager) CurrentMode(context.Context) (profile.AuthMode, error) {
	f.events = append(f.events, "status")
	return f.current, f.currentErr
}

func (f *fakeManager) LoginChatGPT(context.Context) error {
	f.events = append(f.events, "login-chatgpt")
	return f.loginErr
}

func (f *fakeManager) LoginAPI(_ context.Context, apiKey string) error {
	f.events = append(f.events, "login-api")
	f.apiKey = apiKey
	return f.loginErr
}

func (f *fakeManager) Logout(context.Context) error {
	f.events = append(f.events, "logout")
	return f.logoutErr
}

func TestEnsureAuthModeScenarios(t *testing.T) {
	tests := []struct {
		name       string
		current    profile.AuthMode
		target     profile.AuthMode
		apiKey     string
		wantEvents []string
		wantKey    string
		wantErr    error
	}{
		{
			name:       "chatgpt already active",
			current:    profile.ChatGPT,
			target:     profile.ChatGPT,
			wantEvents: []string{"status"},
		},
		{
			name:       "api already active",
			current:    profile.API,
			target:     profile.API,
			wantEvents: []string{"status"},
		},
		{
			name:       "chatgpt to api",
			current:    profile.ChatGPT,
			target:     profile.API,
			apiKey:     "dummy-api-key",
			wantEvents: []string{"status", "logout", "login-api"},
			wantKey:    "dummy-api-key",
		},
		{
			name:       "api to chatgpt",
			current:    profile.API,
			target:     profile.ChatGPT,
			wantEvents: []string{"status", "logout", "login-chatgpt"},
		},
		{
			name:       "none to chatgpt",
			current:    profile.None,
			target:     profile.ChatGPT,
			wantEvents: []string{"status", "login-chatgpt"},
		},
		{
			name:       "none to api",
			current:    profile.None,
			target:     profile.API,
			apiKey:     "dummy-api-key",
			wantEvents: []string{"status", "login-api"},
			wantKey:    "dummy-api-key",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeManager{current: test.current}
			err := EnsureAuthMode(context.Background(), manager, test.target, test.apiKey)
			if err != nil {
				t.Fatalf("EnsureAuthMode returned error: %v", err)
			}
			if !reflect.DeepEqual(manager.events, test.wantEvents) {
				t.Fatalf("events = %v, want %v", manager.events, test.wantEvents)
			}
			if manager.apiKey != test.wantKey {
				t.Fatalf("API key = %q, want %q", manager.apiKey, test.wantKey)
			}
		})
	}
}

func TestEnsureAuthModeMissingAPIKeyDoesNotLogout(t *testing.T) {
	manager := &fakeManager{current: profile.ChatGPT}
	err := EnsureAuthMode(context.Background(), manager, profile.API, " \t")
	if !errors.Is(err, ErrAPIKeyRequired) {
		t.Fatalf("error = %v, want ErrAPIKeyRequired", err)
	}
	if !reflect.DeepEqual(manager.events, []string{"status"}) {
		t.Fatalf("events = %v, want status only", manager.events)
	}
}

func TestEnsureAuthModeUnknownDoesNotLogout(t *testing.T) {
	manager := &fakeManager{current: profile.Unknown}
	err := EnsureAuthMode(context.Background(), manager, profile.API, "dummy-api-key")
	if !errors.Is(err, ErrUnknownAuthMode) {
		t.Fatalf("error = %v, want ErrUnknownAuthMode", err)
	}
	if !reflect.DeepEqual(manager.events, []string{"status"}) {
		t.Fatalf("events = %v, want status only", manager.events)
	}
}

func TestEnsureAuthModeStatusErrorDoesNotLogout(t *testing.T) {
	manager := &fakeManager{currentErr: errors.New("status failed")}
	err := EnsureAuthMode(context.Background(), manager, profile.ChatGPT, "")
	if !errors.Is(err, ErrUnknownAuthMode) {
		t.Fatalf("error = %v, want ErrUnknownAuthMode", err)
	}
	if !reflect.DeepEqual(manager.events, []string{"status"}) {
		t.Fatalf("events = %v, want status only", manager.events)
	}
}

func TestEnsureAuthModeLoginFailureAfterLogoutIsMarked(t *testing.T) {
	loginErr := errors.New("login failed")
	manager := &fakeManager{current: profile.ChatGPT, loginErr: loginErr}
	err := EnsureAuthMode(context.Background(), manager, profile.API, "dummy-api-key")
	if !errors.Is(err, loginErr) || !IsAfterLogout(err) {
		t.Fatalf("error = %v, want login error marked after logout", err)
	}
	if !reflect.DeepEqual(manager.events, []string{"status", "logout", "login-api"}) {
		t.Fatalf("events = %v", manager.events)
	}
}

func TestEnsureAuthModeLogoutFailureDoesNotLogin(t *testing.T) {
	logoutErr := errors.New("logout failed")
	manager := &fakeManager{current: profile.ChatGPT, logoutErr: logoutErr}
	err := EnsureAuthMode(context.Background(), manager, profile.API, "dummy-api-key")
	if !errors.Is(err, logoutErr) || IsAfterLogout(err) {
		t.Fatalf("error = %v, want logout error without after-logout marker", err)
	}
	if !reflect.DeepEqual(manager.events, []string{"status", "logout"}) {
		t.Fatalf("events = %v", manager.events)
	}
}

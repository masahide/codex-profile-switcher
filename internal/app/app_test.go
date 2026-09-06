package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/masahide/codex-profile-switcher/internal/codex"
	"github.com/masahide/codex-profile-switcher/internal/openaiusage"
	"github.com/masahide/codex-profile-switcher/internal/profile"
)

type fakeCodex struct {
	runProfiles    []profile.Profile
	runArgs        [][]string
	loginProfiles  []profile.Profile
	loginError     error
	statusProfiles []profile.Profile
	statusErrors   map[profile.Profile]error
}

func (f *fakeCodex) Run(_ context.Context, p profile.Profile, args []string) error {
	f.runProfiles = append(f.runProfiles, p)
	f.runArgs = append(f.runArgs, append([]string(nil), args...))
	return nil
}

func (f *fakeCodex) Login(_ context.Context, p profile.Profile) error {
	f.loginProfiles = append(f.loginProfiles, p)
	return f.loginError
}

func (f *fakeCodex) LoginStatus(_ context.Context, p profile.Profile) error {
	f.statusProfiles = append(f.statusProfiles, p)
	return f.statusErrors[p]
}

type fakeUsage struct {
	query   openaiusage.UsageQuery
	results []openaiusage.CompletionUsageResult
	err     error
}

func (f *fakeUsage) CompletionUsage(_ context.Context, query openaiusage.UsageQuery) ([]openaiusage.CompletionUsageResult, error) {
	f.query = query
	return f.results, f.err
}

func testApp(codexRunner *fakeCodex, usage *fakeUsage, out, errOut *bytes.Buffer) *App {
	return &App{
		Codex:       codexRunner,
		Usage:       usage,
		Out:         out,
		ErrOut:      errOut,
		ProfileRoot: func() (string, error) { return filepath.Join("root", "codex-profiles"), nil },
		Now:         func() time.Time { return time.Date(2026, 9, 6, 12, 20, 0, 0, time.FixedZone("JST", 9*60*60)) },
		Env: func(key string) string {
			if key == openaiusage.UsageTierEnvironment {
				return ""
			}
			return ""
		},
		Policy: openaiusage.DefaultPolicy(),
	}
}

func TestRunProfilePassesArguments(t *testing.T) {
	codexRunner := &fakeCodex{}
	app := testApp(codexRunner, &fakeUsage{}, new(bytes.Buffer), new(bytes.Buffer))
	args := []string{"exec", "hello world", "--model", "gpt-5.6-sol"}
	if code := app.Run(context.Background(), append([]string{"api"}, args...)); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !reflect.DeepEqual(codexRunner.runProfiles, []profile.Profile{profile.API}) {
		t.Fatalf("profiles = %v", codexRunner.runProfiles)
	}
	if !reflect.DeepEqual(codexRunner.runArgs, [][]string{args}) {
		t.Fatalf("args = %v, want %v", codexRunner.runArgs, args)
	}
}

func TestLoginUsesProfileSpecificFlow(t *testing.T) {
	codexRunner := &fakeCodex{}
	app := testApp(codexRunner, &fakeUsage{}, new(bytes.Buffer), new(bytes.Buffer))
	if code := app.Run(context.Background(), []string{"login", "chatgpt"}); code != 0 {
		t.Fatalf("chatgpt exit code = %d", code)
	}
	if code := app.Run(context.Background(), []string{"login", "api"}); code != 0 {
		t.Fatalf("api exit code = %d", code)
	}
	if !reflect.DeepEqual(codexRunner.loginProfiles, []profile.Profile{profile.ChatGPT, profile.API}) {
		t.Fatalf("login profiles = %v", codexRunner.loginProfiles)
	}
}

func TestAPILoginTTYErrorIncludesPipeInstruction(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{loginError: codex.ErrAPIKeyRequiresStdin}, &fakeUsage{}, out, errOut)
	if code := app.Run(context.Background(), []string{"login", "api"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got := errOut.String(); !strings.Contains(got, "cx: API login requires the API key on stdin") || !strings.Contains(got, "printenv OPENAI_API_KEY | cx login api") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestStatusContinuesAfterOneProfileFails(t *testing.T) {
	codexRunner := &fakeCodex{statusErrors: map[profile.Profile]error{profile.ChatGPT: errors.New("not logged in")}}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(codexRunner, &fakeUsage{}, out, errOut)
	if code := app.Run(context.Background(), []string{"status"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !reflect.DeepEqual(codexRunner.statusProfiles, []profile.Profile{profile.ChatGPT, profile.API}) {
		t.Fatalf("status profiles = %v", codexRunner.statusProfiles)
	}
	if !strings.Contains(errOut.String(), "chatgpt status failed") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestPathAndProfilesAreScriptFriendly(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{}, &fakeUsage{}, out, errOut)
	if code := app.Run(context.Background(), []string{"path", "chatgpt"}); code != 0 {
		t.Fatalf("path exit code = %d", code)
	}
	if got, want := out.String(), filepath.Join("root", "codex-profiles", "chatgpt")+"\n"; got != want {
		t.Fatalf("path output = %q, want %q", got, want)
	}
	out.Reset()
	if code := app.Run(context.Background(), []string{"profiles"}); code != 0 {
		t.Fatalf("profiles exit code = %d", code)
	}
	if got, want := out.String(), "chatgpt\napi\n"; got != want {
		t.Fatalf("profiles output = %q, want %q", got, want)
	}
}

func TestQuotaUsesUTCAndFlagPrecedence(t *testing.T) {
	usage := &fakeUsage{results: []openaiusage.CompletionUsageResult{{Model: "gpt-5.6-sol", InputTokens: 100, OutputTokens: 50}}}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{}, usage, out, errOut)
	app.Env = func(key string) string {
		switch key {
		case openaiusage.UsageTierEnvironment:
			return "4"
		case openaiusage.ProjectIDsEnvironment:
			return "proj_env"
		default:
			return ""
		}
	}
	if code := app.Run(context.Background(), []string{"quota", "--json", "--usage-tier", "2", "--project", "proj_flag", "--project", "proj_other"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if got, want := usage.query.StartTime, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("start = %v, want %v", got, want)
	}
	if got, want := usage.query.EndTime, time.Date(2026, 9, 6, 3, 20, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("end = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(usage.query.ProjectIDs, []string{"proj_flag", "proj_other"}) {
		t.Fatalf("project IDs = %v", usage.query.ProjectIDs)
	}
	var report openaiusage.QuotaReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("JSON unmarshal returned error: %v; output=%q", err, out.String())
	}
	if report.Kind != "estimate" || report.UsageTier == nil || *report.UsageTier != 2 {
		t.Fatalf("report = %+v", report)
	}
	if strings.Contains(out.String(), "admin-key") {
		t.Fatal("secret appeared in JSON output")
	}
}

func TestUsageTierDefaultsToOne(t *testing.T) {
	usage := &fakeUsage{}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{}, usage, out, errOut)
	if code := app.Run(context.Background(), []string{"quota", "--json"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"usage_tier":1`) {
		t.Fatalf("JSON = %q", out.String())
	}
	var report openaiusage.QuotaReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("JSON unmarshal returned error: %v; output=%q", err, out.String())
	}
	large := report.Pools[string(openaiusage.QuotaPoolLarge)]
	if large.Quota == nil || *large.Quota != 250_000 {
		t.Fatalf("default Tier 1 quota = %v, want 250000", large.Quota)
	}
}

func TestInvalidCommandReturnsUsageCode(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{}, &fakeUsage{}, out, errOut)
	if code := app.Run(context.Background(), []string{"unknown"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command or profile: unknown") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

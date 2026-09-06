package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/masahide/codex-profile-switcher/internal/auth"
	"github.com/masahide/codex-profile-switcher/internal/openaiusage"
	"github.com/masahide/codex-profile-switcher/internal/profile"
)

type fakeCodex struct {
	current    profile.AuthMode
	currentErr error
	events     []string
	runArgs    [][]string
	apiKey     string
	loginErr   error
	logoutErr  error
}

func (f *fakeCodex) Run(_ context.Context, args []string) error {
	f.events = append(f.events, "run")
	f.runArgs = append(f.runArgs, append([]string(nil), args...))
	return nil
}

func (f *fakeCodex) CurrentMode(context.Context) (profile.AuthMode, error) {
	f.events = append(f.events, "status")
	return f.current, f.currentErr
}

func (f *fakeCodex) LoginChatGPT(context.Context) error {
	f.events = append(f.events, "login-chatgpt")
	return f.loginErr
}

func (f *fakeCodex) LoginAPI(_ context.Context, apiKey string) error {
	f.events = append(f.events, "login-api")
	f.apiKey = apiKey
	return f.loginErr
}

func (f *fakeCodex) Logout(context.Context) error {
	f.events = append(f.events, "logout")
	return f.logoutErr
}

type fakeUsage struct {
	query     openaiusage.UsageQuery
	results   []openaiusage.CompletionUsageResult
	err       error
	costQuery openaiusage.CostsQuery
	costs     []openaiusage.CostResult
	costErr   error
}

func (f *fakeUsage) CompletionUsage(_ context.Context, query openaiusage.UsageQuery) ([]openaiusage.CompletionUsageResult, error) {
	f.query = query
	return f.results, f.err
}

func (f *fakeUsage) Costs(_ context.Context, query openaiusage.CostsQuery) ([]openaiusage.CostResult, error) {
	f.costQuery = query
	return f.costs, f.costErr
}

func testApp(codexRunner *fakeCodex, usage *fakeUsage, out, errOut *bytes.Buffer) *App {
	return &App{
		Codex:  codexRunner,
		Auth:   codexRunner,
		Usage:  usage,
		Out:    out,
		ErrOut: errOut,
		Now:    func() time.Time { return time.Date(2026, 9, 6, 12, 20, 0, 0, time.FixedZone("JST", 9*60*60)) },
		Env: func(string) string {
			return ""
		},
		Policy: openaiusage.DefaultPolicy(),
	}
}

func TestRunAuthModePassesArguments(t *testing.T) {
	codexRunner := &fakeCodex{current: profile.API}
	app := testApp(codexRunner, &fakeUsage{}, new(bytes.Buffer), new(bytes.Buffer))
	args := []string{"exec", "hello world", "--model", "gpt-5.6-sol"}
	if code := app.Run(context.Background(), append([]string{"api"}, args...)); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !reflect.DeepEqual(codexRunner.events, []string{"status", "run"}) {
		t.Fatalf("events = %v", codexRunner.events)
	}
	if !reflect.DeepEqual(codexRunner.runArgs, [][]string{args}) {
		t.Fatalf("args = %v, want %v", codexRunner.runArgs, args)
	}
}

func TestRunAuthModeSwitchScenarios(t *testing.T) {
	tests := []struct {
		name       string
		current    profile.AuthMode
		target     string
		apiKey     string
		wantEvents []string
		wantKey    string
	}{
		{name: "chatgpt already active", current: profile.ChatGPT, target: "chatgpt", wantEvents: []string{"status", "run"}},
		{name: "api already active", current: profile.API, target: "api", wantEvents: []string{"status", "run"}},
		{name: "chatgpt to api", current: profile.ChatGPT, target: "api", apiKey: "dummy-api-key", wantEvents: []string{"status", "logout", "login-api", "run"}, wantKey: "dummy-api-key"},
		{name: "api to chatgpt", current: profile.API, target: "chatgpt", wantEvents: []string{"status", "logout", "login-chatgpt", "run"}},
		{name: "none to chatgpt", current: profile.None, target: "chatgpt", wantEvents: []string{"status", "login-chatgpt", "run"}},
		{name: "none to api", current: profile.None, target: "api", apiKey: "dummy-api-key", wantEvents: []string{"status", "login-api", "run"}, wantKey: "dummy-api-key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			codexRunner := &fakeCodex{current: test.current}
			out, errOut := new(bytes.Buffer), new(bytes.Buffer)
			app := testApp(codexRunner, &fakeUsage{}, out, errOut)
			app.Env = func(key string) string {
				if key == auth.APIKeyEnvironment {
					return test.apiKey
				}
				return ""
			}
			if code := app.Run(context.Background(), []string{test.target}); code != 0 {
				t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
			}
			if !reflect.DeepEqual(codexRunner.events, test.wantEvents) {
				t.Fatalf("events = %v, want %v", codexRunner.events, test.wantEvents)
			}
			if codexRunner.apiKey != test.wantKey {
				t.Fatalf("API key = %q, want %q", codexRunner.apiKey, test.wantKey)
			}
		})
	}
}

func TestRunAPIWithoutKeyDoesNotLogout(t *testing.T) {
	codexRunner := &fakeCodex{current: profile.ChatGPT}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(codexRunner, &fakeUsage{}, out, errOut)
	if code := app.Run(context.Background(), []string{"api"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !reflect.DeepEqual(codexRunner.events, []string{"status"}) {
		t.Fatalf("events = %v, want status only", codexRunner.events)
	}
	if !strings.Contains(errOut.String(), "CX_OPENAI_API_KEY is required") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunUnknownAuthModeDoesNotStartCodex(t *testing.T) {
	codexRunner := &fakeCodex{current: profile.Unknown}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(codexRunner, &fakeUsage{}, out, errOut)
	if code := app.Run(context.Background(), []string{"chatgpt"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !reflect.DeepEqual(codexRunner.events, []string{"status"}) {
		t.Fatalf("events = %v, want status only", codexRunner.events)
	}
	if !strings.Contains(errOut.String(), "unable to determine current Codex authentication mode") || !strings.Contains(errOut.String(), "codex login status") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunLoginFailureAfterLogoutDoesNotStartCodex(t *testing.T) {
	codexRunner := &fakeCodex{current: profile.ChatGPT, loginErr: errors.New("login failed")}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(codexRunner, &fakeUsage{}, out, errOut)
	app.Env = func(key string) string {
		if key == auth.APIKeyEnvironment {
			return "dummy-api-key"
		}
		return ""
	}
	if code := app.Run(context.Background(), []string{"api"}); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !reflect.DeepEqual(codexRunner.events, []string{"status", "logout", "login-api"}) {
		t.Fatalf("events = %v", codexRunner.events)
	}
	if !strings.Contains(errOut.String(), "failed after logout") || strings.Contains(errOut.String(), "dummy-api-key") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestStatusPrintsNormalizedAuthenticationMode(t *testing.T) {
	tests := []struct {
		mode profile.AuthMode
		want string
	}{
		{mode: profile.ChatGPT, want: "Authentication: ChatGPT\n"},
		{mode: profile.API, want: "Authentication: OpenAI API key\n"},
		{mode: profile.None, want: "Authentication: not logged in\n"},
	}
	for _, test := range tests {
		t.Run(string(test.mode), func(t *testing.T) {
			out, errOut := new(bytes.Buffer), new(bytes.Buffer)
			app := testApp(&fakeCodex{current: test.mode}, &fakeUsage{}, out, errOut)
			if code := app.Run(context.Background(), []string{"status"}); code != 0 {
				t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
			}
			if out.String() != test.want {
				t.Fatalf("output = %q, want %q", out.String(), test.want)
			}
		})
	}
}

func TestStatusRejectsModeArgument(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{current: profile.API}, &fakeUsage{}, out, errOut)
	if code := app.Run(context.Background(), []string{"status", "api"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestQuotaUsesUTCAndFlagPrecedence(t *testing.T) {
	usage := &fakeUsage{results: []openaiusage.CompletionUsageResult{{Model: "gpt-5.6-sol", InputTokens: 100, OutputTokens: 50}}}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{current: profile.API}, usage, out, errOut)
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
	app := testApp(&fakeCodex{current: profile.API}, usage, out, errOut)
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

func TestQuotaShowsBilledCostAndKeepsEstimateWhenCostsFail(t *testing.T) {
	usage := &fakeUsage{
		results: []openaiusage.CompletionUsageResult{{Model: "gpt-5.6-sol", InputTokens: 100, OutputTokens: 50}},
		costs:   []openaiusage.CostResult{{Amount: openaiusage.CostAmount{Value: 0.60, Currency: "usd"}, LineItem: "Input tokens"}, {Amount: openaiusage.CostAmount{Value: 0.50, Currency: "usd"}, LineItem: "Output tokens"}},
	}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{current: profile.API}, usage, out, errOut)
	if code := app.Run(context.Background(), []string{"quota", "--verbose"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Billed cost today") || !strings.Contains(out.String(), "total: $1.10") {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(out.String(), "Input tokens") || !strings.Contains(out.String(), "Output tokens") {
		t.Fatalf("verbose cost line items missing: %q", out.String())
	}
	if !reflect.DeepEqual(usage.costQuery, usage.query) {
		t.Fatalf("cost query = %+v, usage query = %+v", usage.costQuery, usage.query)
	}

	usage.costErr = errors.New("costs unavailable")
	out.Reset()
	errOut.Reset()
	if code := app.Run(context.Background(), []string{"quota"}); code != 0 {
		t.Fatalf("cost failure exit code = %d, stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Large model group") || !strings.Contains(out.String(), "Billed cost today is unavailable: costs unavailable") {
		t.Fatalf("estimate/warning output = %q", out.String())
	}
}

func TestQuotaUsesUsageTierForComplimentaryUsedAndCostsAsSupplement(t *testing.T) {
	usage := &fakeUsage{
		results: []openaiusage.CompletionUsageResult{
			{Model: "gpt-5.6-sol", ServiceTier: "default", InputTokens: 10_000, OutputTokens: 5_000},
			{Model: "gpt-5.6-sol", ServiceTier: openaiusage.IncentivizedServiceTier, InputTokens: 200, OutputTokens: 50},
		},
		costs: []openaiusage.CostResult{{Amount: openaiusage.CostAmount{Value: 999.99, Currency: "usd"}}},
	}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{current: profile.API}, usage, out, errOut)
	if code := app.Run(context.Background(), []string{"quota", "--json"}); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	var report openaiusage.QuotaReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("JSON unmarshal returned error: %v; output=%q", err, out.String())
	}
	large := report.Pools[string(openaiusage.QuotaPoolLarge)]
	if large.EligibleTraffic != 250 {
		t.Fatalf("complimentary used = %d, want 250", large.EligibleTraffic)
	}
	if report.BilledCostToday == nil || report.BilledCostToday.Total != 999.99 {
		t.Fatalf("billed cost = %+v", report.BilledCostToday)
	}
}

func TestInvalidCommandReturnsUsageCode(t *testing.T) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	app := testApp(&fakeCodex{current: profile.API}, &fakeUsage{}, out, errOut)
	if code := app.Run(context.Background(), []string{"unknown"}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command or profile: unknown") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

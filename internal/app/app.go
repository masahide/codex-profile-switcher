// Package app implements cx's command grammar and connects the small domain
// packages without making the CLI framework-dependent.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/masahide/codex-profile-switcher/internal/codex"
	"github.com/masahide/codex-profile-switcher/internal/openaiusage"
	"github.com/masahide/codex-profile-switcher/internal/profile"
)

type CodexRunner interface {
	Run(context.Context, profile.Profile, []string) error
	Login(context.Context, profile.Profile) error
	LoginStatus(context.Context, profile.Profile) error
}

type UsageClient interface {
	CompletionUsage(context.Context, openaiusage.UsageQuery) ([]openaiusage.CompletionUsageResult, error)
}

// App is the testable command handler for cx.
type App struct {
	Codex       CodexRunner
	Usage       UsageClient
	Out         io.Writer
	ErrOut      io.Writer
	ProfileRoot func() (string, error)
	Now         func() time.Time
	Env         func(string) string
	Policy      openaiusage.ComplimentaryPolicy
}

func New(version string) *App {
	return &App{
		Codex:       &codex.Runner{},
		Usage:       openaiusage.NewClient(version),
		Out:         os.Stdout,
		ErrOut:      os.Stderr,
		ProfileRoot: profile.Root,
		Now:         time.Now,
		Env:         os.Getenv,
		Policy:      openaiusage.DefaultPolicy(),
	}
}

// Run executes args and returns cx's process exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return a.help()
	}

	switch args[0] {
	case "-h", "--help":
		if len(args) != 1 {
			return a.usageError("%s does not accept additional arguments", args[0])
		}
		return a.help()
	case "help":
		return a.runHelp(args[1:])
	case "login":
		return a.runLogin(ctx, args[1:])
	case "status":
		return a.runStatus(ctx, args[1:])
	case "path":
		return a.runPath(args[1:])
	case "profiles":
		return a.runProfiles(args[1:])
	case "quota":
		return a.runQuota(ctx, args[1:])
	}

	if p, err := profile.Parse(args[0]); err == nil {
		return a.runCodex(ctx, p, args[1:])
	}
	return a.usageError("unknown command or profile: %s", args[0])
}

func (a *App) runCodex(ctx context.Context, p profile.Profile, args []string) int {
	if a.Codex == nil {
		a.errorf("Codex runner is not configured")
		return 1
	}
	if err := a.Codex.Run(ctx, p, args); err != nil {
		a.errorf("Codex failed: %v", err)
		return codex.ExitCode(err)
	}
	return 0
}

func (a *App) runLogin(ctx context.Context, args []string) int {
	if len(args) != 1 {
		return a.usageError("usage: cx login <profile>")
	}
	p, err := profile.Parse(args[0])
	if err != nil {
		return a.unknownProfile(args[0])
	}
	if a.Codex == nil {
		a.errorf("Codex runner is not configured")
		return 1
	}
	if err := a.Codex.Login(ctx, p); err != nil {
		if errors.Is(err, codex.ErrAPIKeyRequiresStdin) {
			a.errorf("API login requires the API key on stdin\n\n  printenv OPENAI_API_KEY | cx login api")
			return 2
		}
		a.errorf("Codex login failed: %v", err)
		return 1
	}
	return 0
}

func (a *App) runStatus(ctx context.Context, args []string) int {
	var profiles []profile.Profile
	switch len(args) {
	case 0:
		profiles = profile.Builtins()
	case 1:
		p, err := profile.Parse(args[0])
		if err != nil {
			return a.unknownProfile(args[0])
		}
		profiles = []profile.Profile{p}
	default:
		return a.usageError("usage: cx status [profile]")
	}
	if a.Codex == nil {
		a.errorf("Codex runner is not configured")
		return 1
	}

	failed := false
	for _, p := range profiles {
		if err := a.writef("%s:\n", p); err != nil {
			a.errorf("write status output: %v", err)
			return 1
		}
		if err := a.Codex.LoginStatus(ctx, p); err != nil {
			a.errorf("%s status failed: %v", p, err)
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

func (a *App) runPath(args []string) int {
	if len(args) != 1 {
		return a.usageError("usage: cx path <profile>")
	}
	p, err := profile.Parse(args[0])
	if err != nil {
		return a.unknownProfile(args[0])
	}
	rootFunc := a.ProfileRoot
	if rootFunc == nil {
		rootFunc = profile.Root
	}
	root, err := rootFunc()
	if err != nil {
		a.errorf("resolve profile root: %v", err)
		return 1
	}
	path, err := profile.PathFromRoot(root, p)
	if err != nil {
		a.errorf("resolve profile path: %v", err)
		return 1
	}
	if err := a.writef("%s\n", path); err != nil {
		a.errorf("write path: %v", err)
		return 1
	}
	return 0
}

func (a *App) runProfiles(args []string) int {
	if len(args) != 0 {
		return a.usageError("usage: cx profiles")
	}
	for _, p := range profile.Builtins() {
		if err := a.writef("%s\n", p); err != nil {
			a.errorf("write profiles: %v", err)
			return 1
		}
	}
	return 0
}

type quotaOptions struct {
	Verbose      bool
	JSON         bool
	UsageTier    *int
	UsageTierSet bool
	ProjectIDs   []string
	ProjectsSet  bool
}

const defaultUsageTier = 1

func (a *App) runQuota(ctx context.Context, args []string) int {
	options, showHelp, err := parseQuotaOptions(args)
	if err != nil {
		return a.usageError("%v", err)
	}
	if showHelp {
		return a.quotaHelp()
	}

	usageTier, usageTierSource, err := a.resolveUsageTier(options)
	if err != nil {
		return a.usageError("%v", err)
	}
	projectIDs, err := a.resolveProjectIDs(options)
	if err != nil {
		return a.usageError("%v", err)
	}
	nowFunc := a.Now
	if nowFunc == nil {
		nowFunc = time.Now
	}
	now := nowFunc().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	query := openaiusage.UsageQuery{
		StartTime:  start,
		EndTime:    now,
		ProjectIDs: append([]string(nil), projectIDs...),
	}
	if a.Usage == nil {
		a.errorf("Usage API client is not configured")
		return 1
	}
	results, err := a.Usage.CompletionUsage(ctx, query)
	if err != nil {
		a.errorf("%v", err)
		return 1
	}
	policy := a.Policy
	if len(policy.Pools) == 0 {
		policy = openaiusage.DefaultPolicy()
	}
	report := openaiusage.CalculateEstimate(results, policy, usageTier)
	report.Window = openaiusage.Window{
		Start:     start,
		End:       now,
		NextReset: start.Add(24 * time.Hour),
	}
	report.Scope.ProjectIDs = append([]string(nil), projectIDs...)
	if report.Scope.ProjectIDs == nil {
		report.Scope.ProjectIDs = make([]string, 0)
	}
	report.Details.UsageTierSource = usageTierSource

	if options.JSON {
		if err := json.NewEncoder(a.stdout()).Encode(report); err != nil {
			a.errorf("write JSON output: %v", err)
			return 1
		}
		return 0
	}
	if err := openaiusage.FormatText(a.stdout(), report, options.Verbose); err != nil {
		a.errorf("write quota output: %v", err)
		return 1
	}
	return 0
}

func (a *App) resolveUsageTier(options quotaOptions) (*int, string, error) {
	if options.UsageTierSet {
		return cloneInt(options.UsageTier), "flag", nil
	}
	envValue := a.env(openaiusage.UsageTierEnvironment)
	if envValue == "" {
		defaultTier := defaultUsageTier
		return &defaultTier, "default", nil
	}
	tier, err := parseUsageTier(envValue)
	if err != nil {
		return nil, "", fmt.Errorf("invalid %s: %w", openaiusage.UsageTierEnvironment, err)
	}
	return tier, "environment", nil
}

func (a *App) resolveProjectIDs(options quotaOptions) ([]string, error) {
	if options.ProjectsSet {
		return deduplicate(options.ProjectIDs), nil
	}
	return parseProjectIDs(a.env(openaiusage.ProjectIDsEnvironment)), nil
}

func parseQuotaOptions(args []string) (quotaOptions, bool, error) {
	options := quotaOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help", arg == "-h":
			return options, true, nil
		case arg == "--verbose":
			options.Verbose = true
		case arg == "--json":
			options.JSON = true
		case arg == "--usage-tier":
			if i+1 >= len(args) {
				return options, false, errors.New("--usage-tier requires a value")
			}
			i++
			tier, err := parseUsageTier(args[i])
			if err != nil {
				return options, false, fmt.Errorf("invalid --usage-tier: %w", err)
			}
			options.UsageTier = tier
			options.UsageTierSet = true
		case strings.HasPrefix(arg, "--usage-tier="):
			tier, err := parseUsageTier(strings.TrimPrefix(arg, "--usage-tier="))
			if err != nil {
				return options, false, fmt.Errorf("invalid --usage-tier: %w", err)
			}
			options.UsageTier = tier
			options.UsageTierSet = true
		case arg == "--project":
			if i+1 >= len(args) {
				return options, false, errors.New("--project requires a value")
			}
			i++
			projectID := strings.TrimSpace(args[i])
			if projectID == "" {
				return options, false, errors.New("--project requires a non-empty value")
			}
			options.ProjectIDs = append(options.ProjectIDs, projectID)
			options.ProjectsSet = true
		case strings.HasPrefix(arg, "--project="):
			projectID := strings.TrimSpace(strings.TrimPrefix(arg, "--project="))
			if projectID == "" {
				return options, false, errors.New("--project requires a non-empty value")
			}
			options.ProjectIDs = append(options.ProjectIDs, projectID)
			options.ProjectsSet = true
		default:
			return options, false, fmt.Errorf("unknown quota option: %s", arg)
		}
	}
	return options, false, nil
}

func parseUsageTier(value string) (*int, error) {
	tier, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || tier < 1 || tier > 5 {
		return nil, errors.New("must be an integer from 1 to 5")
	}
	return &tier, nil
}

func parseProjectIDs(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		if id := strings.TrimSpace(part); id != "" {
			ids = append(ids, id)
		}
	}
	return deduplicate(ids)
}

func deduplicate(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (a *App) runHelp(args []string) int {
	if len(args) == 0 {
		return a.help()
	}
	if len(args) == 1 && args[0] == "quota" {
		return a.quotaHelp()
	}
	return a.usageError("usage: cx help [quota]")
}

func (a *App) help() int {
	const text = `cx - switch isolated Codex CLI profiles

Usage:
  cx plus [codex args...]
  cx api [codex args...]
  cx login <profile>
  cx status [profile]
  cx path <profile>
  cx profiles
  cx quota [options]
  cx help

Profiles:
  plus    ChatGPT login environment
  api     OpenAI API key login environment

API key login:
  printenv OPENAI_API_KEY | cx login api
  PowerShell: $env:OPENAI_API_KEY | cx login api

The Codex CLI owns authentication and credential storage. cx only sets
CODEX_HOME for the child process; config.toml and local Codex state are also
profile-specific. cx never reads or edits auth.json.

Use "cx quota --help" for the complimentary-token estimate options.
`
	if err := a.write(text); err != nil {
		a.errorf("write help: %v", err)
		return 1
	}
	return 0
}

func (a *App) quotaHelp() int {
	const text = `Usage: cx quota [options]

Estimate today's complimentary-token traffic using the OpenAI Organization
Usage API. This is not an official balance API; the OpenAI Usage Dashboard is
the authoritative place to verify complimentary usage.

Options:
  --verbose                 show model, service-tier, scope, and policy details
  --json                    write machine-readable JSON only
  --usage-tier <1-5>        override CX_OPENAI_USAGE_TIER (default: 1)
  --project <project-id>    restrict the query; may be repeated

Environment:
  OPENAI_ADMIN_KEY          Admin API key, read only for this request
  CX_OPENAI_USAGE_TIER      Usage Tier 1 through 5
  CX_OPENAI_PROJECT_IDS     comma-separated selected project IDs

Official verification:
  https://platform.openai.com/usage/chat-completions
  Enable input/output tokens, group Chat Completions by Service tier, and
  inspect the data sharing incentive tier.
`
	if err := a.write(text); err != nil {
		a.errorf("write quota help: %v", err)
		return 1
	}
	return 0
}

func (a *App) unknownProfile(name string) int {
	return a.usageError("unknown command or profile: %s", name)
}

func (a *App) usageError(format string, args ...interface{}) int {
	a.errorf(format, args...)
	return 2
}

func (a *App) env(key string) string {
	if a.Env != nil {
		return a.Env(key)
	}
	return os.Getenv(key)
}

func (a *App) stdout() io.Writer {
	if a.Out == nil {
		return io.Discard
	}
	return a.Out
}

func (a *App) stderr() io.Writer {
	if a.ErrOut == nil {
		return io.Discard
	}
	return a.ErrOut
}

func (a *App) write(value string) error {
	_, err := io.WriteString(a.stdout(), value)
	return err
}

func (a *App) writef(format string, args ...interface{}) error {
	_, err := fmt.Fprintf(a.stdout(), format, args...)
	return err
}

func (a *App) errorf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(a.stderr(), "cx: "+format+"\n", args...)
}

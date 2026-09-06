// Package openaiusage contains the public Organization Usage API client and
// the local, explicitly estimated complimentary-token calculation.
package openaiusage

import "time"

const (
	DefaultBaseURL         = "https://api.openai.com"
	UsageEndpointPath      = "/v1/organization/usage/completions"
	OfficialDashboardURL   = "https://platform.openai.com/usage/chat-completions"
	AdminAPIKeyEnvironment = "OPENAI_ADMIN_KEY"
	UsageTierEnvironment   = "CX_OPENAI_USAGE_TIER"
	ProjectIDsEnvironment  = "CX_OPENAI_PROJECT_IDS"
)

// UsageQuery describes the time and optional project scope for a usage query.
// The client sends timestamps as Unix seconds, as required by the API.
type UsageQuery struct {
	StartTime  time.Time
	EndTime    time.Time
	ProjectIDs []string
	Page       string
	Limit      int
}

// CompletionUsageResult is one flattened result from a Usage API bucket.
// input_cached_tokens is intentionally retained as a breakdown only; quota
// calculation uses input_tokens + output_tokens once.
type CompletionUsageResult struct {
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	InputCachedTokens int64  `json:"input_cached_tokens"`
	Model             string `json:"model"`
	ProjectID         string `json:"project_id"`
	APIKeyID          string `json:"api_key_id"`
	UserID            string `json:"user_id"`
	Batch             *bool  `json:"batch"`
	ServiceTier       string `json:"service_tier"`
	NumModelRequests  int64  `json:"num_model_requests"`
}

type QuotaPool string

const (
	QuotaPoolLarge QuotaPool = "large"
	QuotaPoolSmall QuotaPool = "small"
)

// PoolPolicy describes one published model group's daily quota.
type PoolPolicy struct {
	Tier12 int64
	Tier35 int64
	Models []string
}

// ComplimentaryPolicy is the versioned policy snapshot used for estimates.
type ComplimentaryPolicy struct {
	SnapshotDate string
	Source       string
	Pools        map[QuotaPool]PoolPolicy
}

// Window is the UTC window represented by a report.
type Window struct {
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	NextReset time.Time `json:"next_reset"`
}

type Scope struct {
	ProjectIDs []string `json:"project_ids"`
}

type PoolEstimate struct {
	Quota                 *int64   `json:"quota"`
	EligibleTraffic       int64    `json:"eligible_traffic"`
	EstimatedRemaining    *int64   `json:"estimated_remaining"`
	EstimatedUsagePercent *float64 `json:"estimated_usage_percent"`
}

// ReportDetails is emitted in verbose text and is kept out of the compact
// JSON shape so that --json remains stable and machine-readable.
type ReportDetails struct {
	UsageTierSource    string
	ModelTraffic       map[string]int64
	EligibleModels     map[string][]string
	ServiceTierTraffic map[string]int64
	UnclassifiedModels []string
	PolicySnapshotDate string
	PolicySource       string
}

// QuotaReport deliberately calls every value an estimate. The API does not
// expose an authoritative complimentary-token balance.
type QuotaReport struct {
	Kind                    string                  `json:"kind"`
	Window                  Window                  `json:"window"`
	UsageTier               *int                    `json:"usage_tier"`
	Scope                   Scope                   `json:"scope"`
	Pools                   map[string]PoolEstimate `json:"pools"`
	OfficialVerificationURL string                  `json:"official_verification_url"`
	Warnings                []string                `json:"warnings"`

	Details ReportDetails `json:"-"`
}

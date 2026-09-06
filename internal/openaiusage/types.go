// Package openaiusage contains the public Organization Usage API client and
// the local, explicitly estimated complimentary-token calculation.
package openaiusage

import "time"

const (
	DefaultBaseURL         = "https://api.openai.com"
	UsageEndpointPath      = "/v1/organization/usage/completions"
	CostsEndpointPath      = "/v1/organization/costs"
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

// CostsQuery is the time and optional project scope for a Costs API query.
// It intentionally has the same shape as UsageQuery because both endpoints
// use the same pagination and organization scope parameters.
type CostsQuery = UsageQuery

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

// CostAmount is the monetary amount returned by the Organization Costs API.
type CostAmount struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// CostResult is one flattened result from a Costs API bucket.
type CostResult struct {
	Amount    CostAmount `json:"amount"`
	LineItem  string     `json:"line_item"`
	ProjectID string     `json:"project_id"`
}

// CostSummary is the actual billed amount returned by the Costs API for the
// requested time window. LineItems is retained for verbose output and JSON
// consumers that want the same breakdown.
type CostSummary struct {
	Total     float64            `json:"total"`
	Currency  string             `json:"currency"`
	LineItems map[string]float64 `json:"line_items,omitempty"`
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

// ReportDetails contains diagnostic data for human-readable output and is
// kept out of the compact JSON shape so that --json remains machine-readable.
type ReportDetails struct {
	UsageTierSource        string
	ModelTraffic           map[string]int64
	EligibleModels         map[string][]string
	ServiceTierTraffic     map[string]int64
	NotCoveredModels       []string
	NotCoveredModelTraffic map[string]int64
	// Deprecated: use NotCoveredModels. This compatibility field is not shown
	// in user-facing output; the wording is intentionally policy-oriented.
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
	BilledCostToday         *CostSummary            `json:"billed_cost_today,omitempty"`
	OfficialVerificationURL string                  `json:"official_verification_url"`
	Warnings                []string                `json:"warnings"`

	Details ReportDetails `json:"-"`
}

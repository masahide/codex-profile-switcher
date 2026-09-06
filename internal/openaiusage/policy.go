package openaiusage

// Complimentary token policy snapshot: 2026-09-06
// Source: https://help.openai.com/en/articles/10306912

const policySnapshotDate = "2026-09-06"
const policySource = "https://help.openai.com/en/articles/10306912"

var largeModels = []string{
	"gpt-5.6-sol",
	"gpt-5.5-2026-04-23",
	"gpt-5.4-2026-03-05",
	"gpt-5.2-2025-12-11",
	"gpt-5.1-2025-11-13",
	"gpt-5.1-codex",
	"gpt-5-codex",
	"gpt-5-2025-08-07",
	"gpt-5-chat-latest",
	"gpt-4.5-preview-2025-02-27",
	"gpt-4.1-2025-04-14",
	"gpt-4o-2024-05-13",
	"gpt-4o-2024-08-06",
	"gpt-4o-2024-11-20",
	"o3-2025-04-16",
	"o1-preview-2024-09-12",
	"o1-2024-12-17",
}

var smallModels = []string{
	"gpt-5.6-terra",
	"gpt-5.6-luna",
	"gpt-5.4-mini-2026-03-17",
	"gpt-5.4-nano-2026-03-17",
	"gpt-5.1-codex-mini",
	"gpt-5-mini-2025-08-07",
	"gpt-5-nano-2025-08-07",
	"gpt-4.1-mini-2025-04-14",
	"gpt-4.1-nano-2025-04-14",
	"gpt-4o-mini-2024-07-18",
	"o4-mini-2025-04-16",
	"o1-mini-2024-09-12",
	"codex-mini-latest",
}

// gpt-6-astra was observed in the Usage API with service_tier
// "incentivized-tier" on 2026-09-06. The Help Center has not been updated
// with this model mapping yet, so keep it as temporary observed data rather
// than adding it to the published model allowlist above.
var observedModelPools = map[string]QuotaPool{
	"gpt-6-astra": QuotaPoolLarge,
}

var modelPools = buildModelPools()

// DefaultPolicy returns the versioned policy used by cx quota.
func DefaultPolicy() ComplimentaryPolicy {
	return ComplimentaryPolicy{
		SnapshotDate: policySnapshotDate,
		Source:       policySource,
		Pools: map[QuotaPool]PoolPolicy{
			QuotaPoolLarge: {
				Tier12: 250_000,
				Tier35: 1_000_000,
				Models: append([]string(nil), largeModels...),
			},
			QuotaPoolSmall: {
				Tier12: 2_500_000,
				Tier35: 10_000_000,
				Models: append([]string(nil), smallModels...),
			},
		},
	}
}

// ClassifyModel returns the static published model-group mapping. This is
// used only to select a quota pool; a model is free-eligible only when its
// Usage API result also has the incentivized service tier.
func ClassifyModel(model string) (QuotaPool, bool) {
	p, ok := modelPools[model]
	return p, ok
}

// classifyModelForEstimate returns the model-group mapping used after the
// service-tier eligibility check. Policy model lists take precedence for
// callers with a custom policy, followed by the temporary observed mapping.
func classifyModelForEstimate(model string, policy ComplimentaryPolicy) (QuotaPool, bool) {
	for _, pool := range []QuotaPool{QuotaPoolLarge, QuotaPoolSmall} {
		poolPolicy, ok := policy.Pools[pool]
		if !ok {
			continue
		}
		for _, candidate := range poolPolicy.Models {
			if candidate == model {
				return pool, true
			}
		}
	}
	if pool, ok := observedModelPools[model]; ok {
		return pool, true
	}
	return ClassifyModel(model)
}

// Limit returns the quota for a valid Usage Tier. Tiers 1-2 and 3-5 share
// their respective published limits.
func (p ComplimentaryPolicy) Limit(pool QuotaPool, usageTier int) (int64, bool) {
	poolPolicy, ok := p.Pools[pool]
	if !ok {
		return 0, false
	}
	switch {
	case usageTier >= 1 && usageTier <= 2:
		return poolPolicy.Tier12, true
	case usageTier >= 3 && usageTier <= 5:
		return poolPolicy.Tier35, true
	default:
		return 0, false
	}
}

func buildModelPools() map[string]QuotaPool {
	pools := make(map[string]QuotaPool, len(largeModels)+len(smallModels))
	for _, model := range largeModels {
		pools[model] = QuotaPoolLarge
	}
	for _, model := range smallModels {
		pools[model] = QuotaPoolSmall
	}
	return pools
}

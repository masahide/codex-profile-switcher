package openaiusage

import (
	"fmt"
	"sort"
)

// CalculateEstimate aggregates Usage API traffic by model and service_tier.
// Only the exact incentivized service tier contributes to complimentary used;
// default and every other tier remain diagnostics and never affect the quota
// estimate. Costs API data is intentionally not an input to this function.
func CalculateEstimate(results []CompletionUsageResult, policy ComplimentaryPolicy, usageTier *int) QuotaReport {
	report := QuotaReport{
		Kind:                    "estimate",
		UsageTier:               cloneIntPointer(usageTier),
		Scope:                   Scope{ProjectIDs: make([]string, 0)},
		Pools:                   make(map[string]PoolEstimate, 2),
		OfficialVerificationURL: OfficialDashboardURL,
		Warnings:                make([]string, 0),
		Details: ReportDetails{
			ModelTraffic:            make(map[string]int64),
			EligibleModels:          make(map[string][]string),
			ServiceTierTraffic:      make(map[string]int64),
			ModelServiceTierTraffic: make(map[string]map[string]int64),
			NotCoveredModels:        make([]string, 0),
			NotCoveredModelTraffic:  make(map[string]int64),
			UnclassifiedModels:      make([]string, 0),
		},
	}

	if policy.SnapshotDate == "" {
		policy.SnapshotDate = policySnapshotDate
	}
	report.Details.PolicySnapshotDate = policy.SnapshotDate
	report.Details.PolicySource = policy.Source

	traffic := map[QuotaPool]int64{
		QuotaPoolLarge: 0,
		QuotaPoolSmall: 0,
	}
	modelSets := map[QuotaPool]map[string]struct{}{
		QuotaPoolLarge: make(map[string]struct{}),
		QuotaPoolSmall: make(map[string]struct{}),
	}
	for _, result := range results {
		tokens := addTokenCounts(result.InputTokens, result.OutputTokens)
		model := displayModel(result.Model)
		report.Details.ModelTraffic[model] = addTokenCounts(report.Details.ModelTraffic[model], tokens)
		tier := displayServiceTier(result.ServiceTier)
		report.Details.ServiceTierTraffic[tier] = addTokenCounts(report.Details.ServiceTierTraffic[tier], tokens)
		modelTiers := report.Details.ModelServiceTierTraffic[model]
		if modelTiers == nil {
			modelTiers = make(map[string]int64)
			report.Details.ModelServiceTierTraffic[model] = modelTiers
		}
		modelTiers[tier] = addTokenCounts(modelTiers[tier], tokens)

		pool, ok := classifyModelForEstimate(result.Model, policy)
		if !ok {
			report.Details.NotCoveredModelTraffic[model] = addTokenCounts(report.Details.NotCoveredModelTraffic[model], tokens)
			continue
		}
		if result.ServiceTier != IncentivizedServiceTier {
			continue
		}
		traffic[pool] = addTokenCounts(traffic[pool], tokens)
		modelSets[pool][result.Model] = struct{}{}
	}

	for model := range report.Details.NotCoveredModelTraffic {
		report.Details.NotCoveredModels = append(report.Details.NotCoveredModels, model)
		report.Details.UnclassifiedModels = append(report.Details.UnclassifiedModels, model)
	}
	sort.Strings(report.Details.NotCoveredModels)
	sort.Strings(report.Details.UnclassifiedModels)
	for _, pool := range []QuotaPool{QuotaPoolLarge, QuotaPoolSmall} {
		models := make([]string, 0, len(modelSets[pool]))
		for model := range modelSets[pool] {
			models = append(models, model)
		}
		sort.Strings(models)
		report.Details.EligibleModels[string(pool)] = models

		estimate := PoolEstimate{EligibleTraffic: traffic[pool]}
		if usageTier != nil {
			if limit, ok := policy.Limit(pool, *usageTier); ok {
				quota := limit
				remaining := limit - traffic[pool]
				if remaining < 0 {
					remaining = 0
				}
				percent := float64(traffic[pool]) / float64(limit) * 100
				estimate.Quota = &quota
				estimate.EstimatedRemaining = &remaining
				estimate.EstimatedUsagePercent = &percent
			}
		}
		report.Pools[string(pool)] = estimate
	}

	return report
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func addTokenCounts(left, right int64) int64 {
	if left < 0 || right < 0 || right > int64(^uint64(0)>>1)-left {
		return int64(^uint64(0) >> 1)
	}
	return left + right
}

func displayModel(model string) string {
	if model == "" {
		return "(unknown model)"
	}
	return model
}

func displayServiceTier(serviceTier string) string {
	if serviceTier == "" {
		return "(unknown)"
	}
	return serviceTier
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (r QuotaReport) validateForOutput() error {
	if r.Kind != "estimate" {
		return fmt.Errorf("unsupported quota report kind %q", r.Kind)
	}
	return nil
}

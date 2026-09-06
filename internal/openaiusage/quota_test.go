package openaiusage

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func result(model, tier string, input, cached, output int64) CompletionUsageResult {
	return CompletionUsageResult{
		Model:             model,
		ServiceTier:       tier,
		InputTokens:       input,
		InputCachedTokens: cached,
		OutputTokens:      output,
	}
}

func TestClassifyModelUsesOnlyPublishedList(t *testing.T) {
	if pool, ok := ClassifyModel("gpt-5.6-sol"); !ok || pool != QuotaPoolLarge {
		t.Fatalf("large classification = %q, %v", pool, ok)
	}
	if pool, ok := ClassifyModel("gpt-5.6-terra"); !ok || pool != QuotaPoolSmall {
		t.Fatalf("small classification = %q, %v", pool, ok)
	}
	if _, ok := ClassifyModel("ft:gpt-5.6-sol:custom"); ok {
		t.Fatal("fine-tuned model was classified")
	}
	if _, ok := ClassifyModel("unknown-model"); ok {
		t.Fatal("unknown model was classified")
	}
}

func TestCalculateEstimateTierTwoLarge(t *testing.T) {
	tier := 2
	report := CalculateEstimate([]CompletionUsageResult{
		result("gpt-5.6-sol", "default", 60_000, 20_000, 40_000),
	}, DefaultPolicy(), &tier)
	large := report.Pools[string(QuotaPoolLarge)]
	if large.Quota == nil || *large.Quota != 250_000 {
		t.Fatalf("quota = %v", large.Quota)
	}
	if large.EligibleTraffic != 100_000 {
		t.Fatalf("traffic = %d, want 100000", large.EligibleTraffic)
	}
	if large.EstimatedRemaining == nil || *large.EstimatedRemaining != 150_000 {
		t.Fatalf("remaining = %v", large.EstimatedRemaining)
	}
	if large.EstimatedUsagePercent == nil || *large.EstimatedUsagePercent != 40 {
		t.Fatalf("usage percent = %v", large.EstimatedUsagePercent)
	}
}

func TestCalculateEstimateTierFourLimits(t *testing.T) {
	tier := 4
	results := []CompletionUsageResult{
		result("gpt-5.6-sol", "default", 50_000, 0, 50_000),
		result("gpt-5.6-terra", "default", 25_000, 10_000, 25_000),
	}
	report := CalculateEstimate(results, DefaultPolicy(), &tier)
	large := report.Pools[string(QuotaPoolLarge)]
	small := report.Pools[string(QuotaPoolSmall)]
	if large.Quota == nil || *large.Quota != 1_000_000 || *large.EstimatedRemaining != 900_000 {
		t.Fatalf("large = %+v", large)
	}
	if small.Quota == nil || *small.Quota != 10_000_000 || small.EligibleTraffic != 50_000 || *small.EstimatedRemaining != 9_950_000 {
		t.Fatalf("small = %+v", small)
	}
}

func TestCalculateEstimateCachedTokensAreNotAdded(t *testing.T) {
	tier := 2
	report := CalculateEstimate([]CompletionUsageResult{
		result("gpt-5.6-sol", "default", 100, 900, 50),
	}, DefaultPolicy(), &tier)
	if got := report.Pools[string(QuotaPoolLarge)].EligibleTraffic; got != 150 {
		t.Fatalf("traffic = %d, want 150", got)
	}
}

func TestCalculateEstimateOverQuotaAndUnknownServiceTier(t *testing.T) {
	tier := 2
	report := CalculateEstimate([]CompletionUsageResult{
		result("gpt-5.6-sol", "", 200_000, 0, 81_000),
		result("unknown-model", "new-tier", 7, 0, 3),
	}, DefaultPolicy(), &tier)
	large := report.Pools[string(QuotaPoolLarge)]
	if large.EstimatedRemaining == nil || *large.EstimatedRemaining != 0 {
		t.Fatalf("remaining = %v, want 0", large.EstimatedRemaining)
	}
	if got := report.Details.ServiceTierTraffic["(unknown)"]; got != 281_000 {
		t.Fatalf("unknown service tier traffic = %d", got)
	}
	if !reflect.DeepEqual(report.Details.UnclassifiedModels, []string{"unknown-model"}) {
		t.Fatalf("unclassified = %v", report.Details.UnclassifiedModels)
	}
}

func TestCalculateEstimateShowsPolicyNotCoveredTrafficWithoutAddingItToQuota(t *testing.T) {
	tier := 2
	report := CalculateEstimate([]CompletionUsageResult{
		result("gpt-6-astra", "default", 500_000, 0, 49_278),
		result("gpt-5.6-sol", "default", 100, 0, 50),
	}, DefaultPolicy(), &tier)

	if _, ok := ClassifyModel("gpt-6-astra"); ok {
		t.Fatal("gpt-6-astra was added to the complimentary-token policy")
	}
	large := report.Pools[string(QuotaPoolLarge)]
	if large.EligibleTraffic != 150 {
		t.Fatalf("large traffic = %d, want 150", large.EligibleTraffic)
	}
	small := report.Pools[string(QuotaPoolSmall)]
	if small.EligibleTraffic != 0 {
		t.Fatalf("small traffic = %d, want 0", small.EligibleTraffic)
	}
	if got := report.Details.NotCoveredModelTraffic["gpt-6-astra"]; got != 549_278 {
		t.Fatalf("not-covered traffic = %d, want 549278", got)
	}
	if !reflect.DeepEqual(report.Details.NotCoveredModels, []string{"gpt-6-astra"}) {
		t.Fatalf("not-covered models = %v", report.Details.NotCoveredModels)
	}
}

func TestCalculateEstimateUnknownUsageTierStillReportsTraffic(t *testing.T) {
	report := CalculateEstimate([]CompletionUsageResult{
		result("gpt-5.6-sol", "default", 100, 0, 83),
	}, DefaultPolicy(), nil)
	large := report.Pools[string(QuotaPoolLarge)]
	if large.EligibleTraffic != 183 {
		t.Fatalf("traffic = %d", large.EligibleTraffic)
	}
	if large.Quota != nil || large.EstimatedRemaining != nil || large.EstimatedUsagePercent != nil {
		t.Fatalf("unknown tier estimate = %+v", large)
	}
}

func TestQuotaReportJSONUsesEstimateAndNulls(t *testing.T) {
	report := CalculateEstimate(nil, DefaultPolicy(), nil)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	text := string(data)
	if !reflect.DeepEqual(report.Kind, "estimate") {
		t.Fatalf("kind = %q", report.Kind)
	}
	if !containsAll(text, []string{`"kind":"estimate"`, `"quota":null`, `"estimated_remaining":null`, `"official_verification_url"`, `"warnings":[]`}) {
		t.Fatalf("JSON = %s", text)
	}
}

func containsAll(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

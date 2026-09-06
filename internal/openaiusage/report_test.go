package openaiusage

import (
	"strings"
	"testing"
)

func TestFormatTextAlwaysShowsPolicySnapshot(t *testing.T) {
	report := CalculateEstimate(nil, DefaultPolicy(), nil)
	var output strings.Builder
	if err := FormatText(&output, report, false); err != nil {
		t.Fatalf("FormatText returned error: %v", err)
	}
	if got := strings.Count(output.String(), "Policy snapshot: "+policySnapshotDate); got != 1 {
		t.Fatalf("policy snapshot count = %d, output=%q", got, output.String())
	}
}

func TestFormatTextDoesNotDuplicatePolicySnapshotInVerboseMode(t *testing.T) {
	report := CalculateEstimate(nil, DefaultPolicy(), nil)
	var output strings.Builder
	if err := FormatText(&output, report, true); err != nil {
		t.Fatalf("FormatText returned error: %v", err)
	}
	if got := strings.Count(output.String(), "Policy snapshot: "+policySnapshotDate); got != 1 {
		t.Fatalf("verbose policy snapshot count = %d, output=%q", got, output.String())
	}
}

func TestFormatTextShowsPolicyNotCoveredModelsInNormalOutput(t *testing.T) {
	report := CalculateEstimate([]CompletionUsageResult{{
		Model:        "unknown-model",
		ServiceTier:  IncentivizedServiceTier,
		InputTokens:  500_000,
		OutputTokens: 49_278,
	}}, DefaultPolicy(), nil)
	var output strings.Builder
	if err := FormatText(&output, report, false); err != nil {
		t.Fatalf("FormatText returned error: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "Not covered by current complimentary-token policy") {
		t.Fatalf("not-covered heading missing: %q", text)
	}
	if !strings.Contains(text, "unknown-model") || !strings.Contains(text, "549,278 tokens") {
		t.Fatalf("not-covered model traffic missing: %q", text)
	}
	if strings.Contains(text, "Unclassified models") {
		t.Fatalf("legacy unclassified wording appeared: %q", text)
	}
}

func TestFormatTextShowsModelServiceTierBreakdownInVerboseOutput(t *testing.T) {
	report := CalculateEstimate([]CompletionUsageResult{
		{Model: "gpt-5.6-sol", ServiceTier: "default", InputTokens: 90, OutputTokens: 10},
		{Model: "gpt-5.6-sol", ServiceTier: IncentivizedServiceTier, InputTokens: 180, OutputTokens: 20},
	}, DefaultPolicy(), nil)
	var output strings.Builder
	if err := FormatText(&output, report, true); err != nil {
		t.Fatalf("FormatText returned error: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "Model/service-tier traffic:") {
		t.Fatalf("model/service-tier heading missing: %q", text)
	}
	if !strings.Contains(text, "gpt-5.6-sol") || !strings.Contains(text, "default") || !strings.Contains(text, "100 tokens") {
		t.Fatalf("default model/service-tier detail missing: %q", text)
	}
	if !strings.Contains(text, IncentivizedServiceTier) || !strings.Contains(text, "200 tokens") {
		t.Fatalf("incentivized model/service-tier detail missing: %q", text)
	}
	if !strings.Contains(text, "complimentary used:") {
		t.Fatalf("complimentary used label missing: %q", text)
	}
}

func TestFormatTextShowsBilledCostAndVerboseLineItems(t *testing.T) {
	report := CalculateEstimate(nil, DefaultPolicy(), nil)
	report.BilledCostToday = &CostSummary{
		Total:    1.10,
		Currency: "usd",
		LineItems: map[string]float64{
			"Output tokens": 0.50,
			"Input tokens":  0.60,
		},
	}

	var normal strings.Builder
	if err := FormatText(&normal, report, false); err != nil {
		t.Fatalf("normal FormatText returned error: %v", err)
	}
	if !strings.Contains(normal.String(), "Billed cost today") || !strings.Contains(normal.String(), "total: $1.10") {
		t.Fatalf("normal billed cost missing: %q", normal.String())
	}
	if strings.Contains(normal.String(), "Input tokens") || strings.Contains(normal.String(), "Output tokens") {
		t.Fatalf("line items appeared in normal output: %q", normal.String())
	}

	var verbose strings.Builder
	if err := FormatText(&verbose, report, true); err != nil {
		t.Fatalf("verbose FormatText returned error: %v", err)
	}
	if !strings.Contains(verbose.String(), "line items:") || !strings.Contains(verbose.String(), "Input tokens") || !strings.Contains(verbose.String(), "Output tokens") {
		t.Fatalf("verbose billed line items missing: %q", verbose.String())
	}
}

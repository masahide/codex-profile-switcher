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

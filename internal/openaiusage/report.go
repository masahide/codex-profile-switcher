package openaiusage

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FormatText writes the human-readable estimate. It intentionally uses
// conservative wording and points users to the official dashboard.
func FormatText(w io.Writer, report QuotaReport, verbose bool) error {
	if err := report.validateForOutput(); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "OpenAI complimentary token estimate"); err != nil {
		return err
	}
	if !report.Window.Start.IsZero() && !report.Window.End.IsZero() {
		if _, err := fmt.Fprintf(w, "Window: %s - %s\n", formatUTC(report.Window.Start), formatUTC(report.Window.End)); err != nil {
			return err
		}
	}
	if report.UsageTier == nil {
		if _, err := fmt.Fprintln(w, "Usage Tier: unknown"); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "Usage Tier: %d\n", *report.UsageTier); err != nil {
		return err
	}
	if verbose && report.Details.UsageTierSource != "" {
		if _, err := fmt.Fprintf(w, "Usage Tier source: %s\n", report.Details.UsageTierSource); err != nil {
			return err
		}
	}
	if len(report.Scope.ProjectIDs) == 0 {
		if _, err := fmt.Fprintln(w, "Scope: entire organization"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "Scope:"); err != nil {
			return err
		}
		for _, projectID := range report.Scope.ProjectIDs {
			if _, err := fmt.Fprintf(w, "  %s\n", projectID); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	for _, pool := range []QuotaPool{QuotaPoolLarge, QuotaPoolSmall} {
		if err := formatPool(w, report, pool); err != nil {
			return err
		}
	}
	if !report.Window.NextReset.IsZero() {
		if _, err := fmt.Fprintf(w, "Next reset: %s\n", formatUTC(report.Window.NextReset)); err != nil {
			return err
		}
	}

	if verbose {
		if err := formatDetails(w, report); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nOfficial verification:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  %s\n", report.OfficialVerificationURL); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\nNote:"); err != nil {
		return err
	}
	notes := []string{
		"Remaining values are estimates derived from the Organization Usage API",
		"and the published complimentary-token policy.",
		"Confirm actual complimentary usage in the OpenAI Usage Dashboard.",
		"A request that crosses the quota may be billed in full; remaining is not a guarantee.",
	}
	for _, note := range notes {
		if _, err := fmt.Fprintf(w, "  %s\n", note); err != nil {
			return err
		}
	}
	if report.UsageTier == nil {
		if _, err := fmt.Fprintln(w, "\nSet CX_OPENAI_USAGE_TIER or use --usage-tier."); err != nil {
			return err
		}
	}
	for _, warning := range report.Warnings {
		if _, err := fmt.Fprintf(w, "Warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func formatPool(w io.Writer, report QuotaReport, pool QuotaPool) error {
	name := "Large model group"
	if pool == QuotaPoolSmall {
		name = "Small model group"
	}
	estimate := report.Pools[string(pool)]
	if _, err := fmt.Fprintln(w, name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  quota:                 %s\n", formatOptionalInt(estimate.Quota)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  eligible traffic:      %s\n", formatInt(estimate.EligibleTraffic)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  estimated remaining:   %s\n", formatOptionalInt(estimate.EstimatedRemaining)); err != nil {
		return err
	}
	if estimate.EstimatedUsagePercent == nil {
		if _, err := fmt.Fprintln(w, "  estimated usage:       unknown"); err != nil {
			return err
		}
	} else if _, err := fmt.Fprintf(w, "  estimated usage:       %.1f%%\n", *estimate.EstimatedUsagePercent); err != nil {
		return err
	}
	if estimate.Quota != nil && estimate.EstimatedRemaining != nil && *estimate.EstimatedRemaining == 0 && estimate.EligibleTraffic >= *estimate.Quota {
		if _, err := fmt.Fprintln(w, "  status:                quota likely exhausted"); err != nil {
			return err
		}
	}
	if models := report.Details.EligibleModels[string(pool)]; len(models) > 0 {
		if _, err := fmt.Fprintf(w, "  includes: %s\n", strings.Join(models, ", ")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	return nil
}

func formatDetails(w io.Writer, report QuotaReport) error {
	if _, err := fmt.Fprintf(w, "Policy snapshot: %s\n", report.Details.PolicySnapshotDate); err != nil {
		return err
	}
	if report.Details.PolicySource != "" {
		if _, err := fmt.Fprintf(w, "Policy source: %s\n", report.Details.PolicySource); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\nModel traffic:"); err != nil {
		return err
	}
	if err := formatSortedCounts(w, report.Details.ModelTraffic); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Raw service tiers:"); err != nil {
		return err
	}
	if err := formatSortedCounts(w, report.Details.ServiceTierTraffic); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Unclassified models:"); err != nil {
		return err
	}
	if len(report.Details.UnclassifiedModels) == 0 {
		if _, err := fmt.Fprintln(w, "  none"); err != nil {
			return err
		}
	} else {
		for _, model := range report.Details.UnclassifiedModels {
			if _, err := fmt.Fprintf(w, "  %s\n", model); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatSortedCounts(w io.Writer, counts map[string]int64) error {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		_, err := fmt.Fprintln(w, "  none")
		return err
	}
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "  %-32s %s\n", key, formatInt(counts[key])); err != nil {
			return err
		}
	}
	return nil
}

func formatUTC(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04") + " UTC"
}

func formatInt(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	digits := strconv.FormatInt(value, 10)
	if len(digits) <= 3 {
		if negative {
			return "-" + digits
		}
		return digits
	}

	first := len(digits) % 3
	if first == 0 {
		first = 3
	}
	var builder strings.Builder
	if negative {
		builder.WriteByte('-')
	}
	builder.WriteString(digits[:first])
	for i := first; i < len(digits); i += 3 {
		builder.WriteByte(',')
		builder.WriteString(digits[i : i+3])
	}
	return builder.String()
}

func formatOptionalInt(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return formatInt(*value)
}

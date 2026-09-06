package openaiusage

import "strings"

// SummarizeCosts aggregates the Costs API results for the requested window.
// The API may return one result per bucket and line item, so both the total
// and each line item are summed here.
func SummarizeCosts(results []CostResult) CostSummary {
	summary := CostSummary{}
	for _, result := range results {
		if summary.Currency == "" {
			summary.Currency = strings.ToLower(strings.TrimSpace(result.Amount.Currency))
		}
		summary.Total += result.Amount.Value

		lineItem := strings.TrimSpace(result.LineItem)
		if lineItem == "" {
			lineItem = "(unspecified)"
		}
		if summary.LineItems == nil {
			summary.LineItems = make(map[string]float64)
		}
		summary.LineItems[lineItem] += result.Amount.Value
	}
	return summary
}

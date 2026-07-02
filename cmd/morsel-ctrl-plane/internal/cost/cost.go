// Package cost provides pure functions for computing platform cost estimates.
package cost

import (
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
)

// BillingPeriodStart returns the first instant of the calendar month containing t, in UTC.
func BillingPeriodStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// BillingPeriodKey returns a "YYYY-MM" string that identifies t's billing period.
func BillingPeriodKey(t time.Time) string {
	return t.UTC().Format("2006-01")
}

// NextBillingPeriodStart returns the first instant of the calendar month after t, in UTC.
func NextBillingPeriodStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}

// RunningHours computes the total hours the app was in a running state during the
// billing period defined by [periodStart, now). events must be ordered by
// occurred_at ascending. hibernated is the current hibernation state of the app.
func RunningHours(events []store.ScaleEvent, periodStart, now time.Time, hibernated bool) float64 {
	var total time.Duration
	var runStart time.Time

	// If the first event this period is scale_to_0, the app was already running at
	// period start. Similarly, no events and not hibernated means running throughout.
	firstIsScaleTo0 := len(events) > 0 && events[0].Event == "scale_to_0"
	noEventsRunning := len(events) == 0 && !hibernated
	if firstIsScaleTo0 || noEventsRunning {
		runStart = periodStart
	}

	for _, e := range events {
		t := e.OccurredAt
		switch e.Event {
		case "scale_to_1":
			if runStart.IsZero() {
				runStart = t
			}
		case "scale_to_0":
			if !runStart.IsZero() {
				total += t.Sub(runStart)
				runStart = time.Time{}
			}
		}
	}
	if !runStart.IsZero() {
		total += now.Sub(runStart)
	}
	return total.Hours()
}

// EstimateAppCostMonthly computes the projected monthly USD cost for an app given
// its tier limits and the total running hours this billing period.
func EstimateAppCostMonthly(tier store.Tier, hours float64, prices store.PriceSnapshot) float64 {
	cpuCores := float64(tier.CpuMilli) / 1000.0
	memGB := float64(tier.MemoryMb) / 1024.0
	computeCost := hours * (cpuCores*prices.ComputeCpuPerCoreHour + memGB*prices.ComputeMemPerGbHour)
	storageCost := float64(tier.BlobGb)*prices.StoragePerGbMonth +
		float64(tier.DatabaseGb)*prices.StoragePerGbMonth
	return computeCost + storageCost
}

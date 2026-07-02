package handler

import (
	"context"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
)

// billingPeriodStart returns the first instant of the current calendar month in UTC.
func billingPeriodStart(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// runningHours computes the total hours the app was in a running state during the billing
// period defined by [periodStart, now). events must be ordered by occurred_at ascending.
// hibernated is the current hibernation state of the app (used when no events exist).
func runningHours(events []store.ScaleEvent, periodStart, now time.Time, hibernated bool) float64 {
	var total time.Duration
	var runStart time.Time

	// If the first event this period is scale_to_0, the app was already running at period
	// start. Similarly, if there are no events and the app is not hibernated, it has been
	// running throughout the period.
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

// estimateAppCostMonthly computes the projected monthly USD cost for an app given its
// tier limits and the total running hours this billing period.
func estimateAppCostMonthly(tier store.Tier, hours float64, prices store.PriceSnapshot) float64 {
	cpuCores := float64(tier.CpuMilli) / 1000.0
	memGB := float64(tier.MemoryMb) / 1024.0
	computeCost := hours * (cpuCores*prices.ComputeCpuPerCoreHour + memGB*prices.ComputeMemPerGbHour)
	storageCost := float64(tier.BlobGb)*prices.StoragePerGbMonth +
		float64(tier.DatabaseGb)*prices.StoragePerGbMonth
	return computeCost + storageCost
}

// appCostMonthly fetches scale events for one app and returns its estimated monthly cost.
func (h *Handler) appCostMonthly(ctx context.Context, app store.App, tier store.Tier, prices store.PriceSnapshot, now time.Time) float64 {
	if !app.Namespace.Valid || app.Namespace.String == "" {
		return 0
	}
	periodStart := billingPeriodStart(now)
	events, err := h.store.ListScaleEventsSince(ctx, app.Namespace.String, app.Name, periodStart)
	if err != nil {
		return 0
	}
	hours := runningHours(events, periodStart, now, app.Hibernated != 0)
	return estimateAppCostMonthly(tier, hours, prices)
}

// repoCostMonthly computes the total estimated monthly cost for all apps in a repo.
func (h *Handler) repoCostMonthly(ctx context.Context, repoSlug string, prices store.PriceSnapshot, now time.Time) float64 {
	apps, err := h.store.ListApps(ctx, repoSlug)
	if err != nil {
		return 0
	}
	repo, err := h.store.GetRepo(ctx, repoSlug)
	if err != nil {
		return 0
	}
	tier, err := h.store.GetTier(ctx, repo.Tier)
	if err != nil {
		return 0
	}
	var total float64
	for _, app := range apps {
		total += h.appCostMonthly(ctx, app, tier, prices, now)
	}
	return total
}

// latestPrices returns the latest price snapshot, or a zero-valued one if none exists yet.
func (h *Handler) latestPrices(ctx context.Context) store.PriceSnapshot {
	snap, err := h.store.GetLatestPriceSnapshot(ctx)
	if err != nil {
		return store.PriceSnapshot{}
	}
	return snap
}

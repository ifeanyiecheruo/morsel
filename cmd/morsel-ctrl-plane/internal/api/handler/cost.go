package handler

import (
	"context"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/cost"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
)

// appCostMonthly fetches scale events for one app and returns its estimated monthly cost.
func (h *Handler) appCostMonthly(ctx context.Context, app store.App, tier store.Tier, prices store.PriceSnapshot, now time.Time) float64 {
	return cost.AppCostMonthly(ctx, h.store, app, tier, prices, now)
}

// repoCostMonthly computes the total estimated monthly cost for all apps in a repo.
func (h *Handler) repoCostMonthly(ctx context.Context, repoSlug string, prices store.PriceSnapshot, now time.Time) float64 {
	return cost.RepoCostMonthly(ctx, h.store, repoSlug, prices, now)
}

// latestPrices returns the latest price snapshot, or a zero-valued one if none exists yet.
func (h *Handler) latestPrices(ctx context.Context) store.PriceSnapshot {
	return cost.LatestPrices(ctx, h.store)
}

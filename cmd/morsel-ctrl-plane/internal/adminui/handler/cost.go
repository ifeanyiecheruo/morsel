package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

type apiCostResponse struct {
	EstimatedMonthly float64 `json:"estimated_monthly"`
	ByRepo           []struct {
		Slug             string  `json:"slug"`
		EstimatedMonthly float64 `json:"estimated_monthly"`
	} `json:"by_repo"`
}

// ServeCost handles GET /cost.
func (h *Handler) ServeCost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp, err := h.apiGet(ctx, "/api/operator/cost")
	if err != nil || resp.StatusCode == http.StatusUnauthorized {
		if resp != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				ctxlog.From(ctx).Warn("close response body", "err", closeErr)
			}
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var costData apiCostResponse
	if err := json.NewDecoder(resp.Body).Decode(&costData); err != nil {
		ctxlog.From(ctx).Warn("decode cost response", "err", err)
	}

	// Get platform config for budget ceiling.
	cfgResp, cfgErr := h.apiGet(ctx, "/api/operator/config")
	var budget float64
	if cfgErr == nil && cfgResp.StatusCode == http.StatusOK {
		defer func() {
			if closeErr := cfgResp.Body.Close(); closeErr != nil {
				ctxlog.From(ctx).Warn("close response body", "err", closeErr)
			}
		}()
		var cfg struct {
			BudgetCeilingMonthly float64 `json:"budget_ceiling_monthly"`
		}
		if err := json.NewDecoder(cfgResp.Body).Decode(&cfg); err != nil {
			ctxlog.From(ctx).Warn("decode config response", "err", err)
		}
		budget = cfg.BudgetCeilingMonthly
	} else if cfgResp != nil {
		if closeErr := cfgResp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}

	byRepo := make([]pages.CostRepoRow, 0, len(costData.ByRepo))
	for _, r := range costData.ByRepo {
		byRepo = append(byRepo, pages.CostRepoRow{
			Slug: r.Slug,
			Cost: r.EstimatedMonthly,
		})
	}

	if err := pages.CostPage(pages.CostPageData{
		TotalCost: costData.EstimatedMonthly,
		Budget:    budget,
		ByRepo:    byRepo,
	}).Render(ctx, w); err != nil {
		ctxlog.From(ctx).Warn("render cost page", "err", err)
	}
}

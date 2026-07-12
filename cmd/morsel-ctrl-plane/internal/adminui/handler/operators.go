package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

type operatorPrincipalDetail struct {
	GithubLogin string `json:"github_login"`
	IsOperator  bool   `json:"is_operator"`
	IsAdmin     bool   `json:"is_admin"`
}

type operatorPrincipalsResponse struct {
	Principals []operatorPrincipalDetail `json:"principals"`
}

// ServeOperators handles GET /operators.
func (h *Handler) ServeOperators(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	resp, err := h.apiGet(ctx, "/api/operator/principals")
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				ctxlog.From(ctx).Warn("close response body", "err", closeErr)
			}
		}
		if err := pages.OperatorsPage(pages.OperatorsPageData{Flash: "Failed to load operators."}).Render(ctx, w); err != nil {
			ctxlog.From(ctx).Warn("render operators page", "err", err)
		}
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var body operatorPrincipalsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		if err := pages.OperatorsPage(pages.OperatorsPageData{Flash: "Failed to parse response."}).Render(ctx, w); err != nil {
			ctxlog.From(ctx).Warn("render operators page", "err", err)
		}
		return
	}

	rows := make([]pages.OperatorRow, len(body.Principals))
	for i, p := range body.Principals {
		rows[i] = pages.OperatorRow{
			GithubLogin: p.GithubLogin,
			IsOperator:  p.IsOperator,
			IsAdmin:     p.IsAdmin,
		}
	}
	if err := pages.OperatorsPage(pages.OperatorsPageData{
		Rows:          rows,
		ViewerIsAdmin: sessionIsAdmin(ctx),
	}).Render(ctx, w); err != nil {
		ctxlog.From(ctx).Warn("render operators page", "err", err)
	}
}

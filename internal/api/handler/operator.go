package handler

import (
	"context"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

// ── Operator stubs ────────────────────────────────────────────────────────────

func (h *Handler) GetOperatorConfig(ctx context.Context) (oas.GetOperatorConfigRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) UpdateOperatorConfig(ctx context.Context, _ *oas.PlatformConfig) (oas.UpdateOperatorConfigRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) UpdateRepoTier(ctx context.Context, _ *oas.UpdateRepoTierReq, _ oas.UpdateRepoTierParams) (oas.UpdateRepoTierRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) ListOperatorApprovals(ctx context.Context) (oas.ListOperatorApprovalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetOperatorApproval(ctx context.Context, _ oas.GetOperatorApprovalParams) (oas.GetOperatorApprovalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) BatchActionApprovals(ctx context.Context, _ *oas.BatchActionApprovalsReq) (oas.BatchActionApprovalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetOperatorCost(ctx context.Context) (oas.GetOperatorCostRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetOperatorStatus(ctx context.Context) (oas.GetOperatorStatusRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

// ── Operator principals ───────────────────────────────────────────────────────

func (h *Handler) ListOperatorPrincipals(ctx context.Context) (oas.ListOperatorPrincipalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	principals, err := h.secretMgr.OperatorPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	return &oas.OperatorPrincipals{Principals: principals}, nil
}

func (h *Handler) AddOperatorPrincipal(ctx context.Context, req *oas.PrincipalReq) (oas.AddOperatorPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	existing, err := h.secretMgr.OperatorPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	// Filter out any existing occurrences, then append exactly once.
	// len(updated) != len(existing) when the list actually changed (new entry or duplicates cleaned up).
	updated := make([]string, 0, len(existing)+1)
	for _, p := range existing {
		if p != req.Principal {
			updated = append(updated, p)
		}
	}
	updated = append(updated, req.Principal)
	if len(updated) != len(existing) {
		if err := h.secretMgr.SetOperatorPrincipals(ctx, updated); err != nil {
			return nil, fmt.Errorf("update principals: %w", err)
		}
	}
	return &oas.OperatorPrincipals{Principals: updated}, nil
}

func (h *Handler) RemoveOperatorPrincipal(ctx context.Context, params oas.RemoveOperatorPrincipalParams) (oas.RemoveOperatorPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	existing, err := h.secretMgr.OperatorPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	updated := make([]string, 0, len(existing))
	for _, p := range existing {
		if p != params.Principal {
			updated = append(updated, p)
		}
	}
	if len(updated) < len(existing) {
		if err := h.secretMgr.SetOperatorPrincipals(ctx, updated); err != nil {
			return nil, fmt.Errorf("update principals: %w", err)
		}
	}
	return &oas.OperatorPrincipals{Principals: updated}, nil
}

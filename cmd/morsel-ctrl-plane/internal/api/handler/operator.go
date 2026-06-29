package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// ── Operator stubs ────────────────────────────────────────────────────────────

func (h *Handler) GetOperatorConfig(ctx context.Context) (server.GetOperatorConfigRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) UpdateOperatorConfig(ctx context.Context, _ *server.PlatformConfig) (server.UpdateOperatorConfigRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) UpdateRepoTier(ctx context.Context, _ *server.UpdateRepoTierReq, _ server.UpdateRepoTierParams) (server.UpdateRepoTierRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) ListOperatorApprovals(ctx context.Context) (server.ListOperatorApprovalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetOperatorApproval(ctx context.Context, _ server.GetOperatorApprovalParams) (server.GetOperatorApprovalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) BatchActionApprovals(ctx context.Context, _ *server.BatchActionApprovalsReq) (server.BatchActionApprovalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetOperatorCost(ctx context.Context) (server.GetOperatorCostRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetDeploymentInfo(ctx context.Context) (server.GetDeploymentInfoRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return &server.DeploymentInfo{Namespace: h.plat.Namespace(), Platform: h.plat.Name()}, nil
}

func (h *Handler) GetOperatorStatus(ctx context.Context) (server.GetOperatorStatusRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}

	expiry, err := h.deployer.GetTLSCertExpiry(ctx, h.plat.Namespace(), kube.MorselTLSSecret)
	if err != nil {
		return nil, fmt.Errorf("get tls cert expiry: %w", err)
	}

	resp := &server.GetOperatorStatusOK{}
	if expiry != nil && time.Until(*expiry) < 30*24*time.Hour {
		resp.Certs = server.NewOptGetOperatorStatusOKCerts(server.GetOperatorStatusOKCerts{
			ExpiringSoon: []string{"*." + h.plat.BaseDomain()},
		})
	}
	return resp, nil
}

// ── Operator principals ───────────────────────────────────────────────────────

func (h *Handler) ListOperatorPrincipals(ctx context.Context) (server.ListOperatorPrincipalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	principals, err := h.store.ListPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	return &server.OperatorPrincipals{Principals: principals}, nil
}

func (h *Handler) AddOperatorPrincipal(ctx context.Context, req *server.PrincipalReq) (server.AddOperatorPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	if err := h.store.AddPrincipal(ctx, req.Principal); err != nil {
		return nil, fmt.Errorf("add principal: %w", err)
	}
	principals, err := h.store.ListPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	return &server.OperatorPrincipals{Principals: principals}, nil
}

func (h *Handler) RemoveOperatorPrincipal(ctx context.Context, params server.RemoveOperatorPrincipalParams) (server.RemoveOperatorPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	if err := h.store.RemovePrincipal(ctx, params.Principal); err != nil {
		return nil, fmt.Errorf("remove principal: %w", err)
	}
	principals, err := h.store.ListPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	return &server.OperatorPrincipals{Principals: principals}, nil
}

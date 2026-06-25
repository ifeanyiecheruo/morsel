package handler

import (
	"context"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

func (h *Handler) PrepareRepoDeploy(ctx context.Context, params oas.PrepareRepoDeployParams) (oas.PrepareRepoDeployRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}

	slug := repoSlug(params.Org, params.Repo)
	deployToken, err := tokens.IssueToken(h.signingKey, tokens.CreateDeployClaims(slug))
	if err != nil {
		return nil, fmt.Errorf("issue deploy token: %w", err)
	}

	return &oas.DeployConfig{
		Token:    deployToken,
		Registry: h.plat.Deploy().StagingRegistry(),
	}, nil
}

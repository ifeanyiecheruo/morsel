package handler

import (
	"context"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
)

func (h *Handler) PrepareRepoDeploy(ctx context.Context, params server.PrepareRepoDeployParams) (server.PrepareRepoDeployRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}

	slug := repoSlug(params.Org, params.Repo)
	deployToken, err := tokens.IssueToken(h.signingKey, tokens.CreateDeployClaims(slug))
	if err != nil {
		return nil, fmt.Errorf("issue deploy token: %w", err)
	}

	return &server.DeployConfig{
		Token:    deployToken,
		Registry: h.plat.Deploy().StagingRegistry(),
	}, nil
}

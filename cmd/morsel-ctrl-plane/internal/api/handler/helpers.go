package handler

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	dbqueries "github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/db/queries"
)

const retryAfterDeploy = 5 * time.Second

func newOperationID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "op_" + hex.EncodeToString(b), nil
}

// appNamespace derives the Kubernetes namespace for an app.
// Formula: {org-slug}-{repo-slug}--{app-name} for named apps,
//
//	{org-slug}-{repo-slug} for unnamed apps.
//
// Slashes and underscores in org/repo/name are replaced with hyphens.
func appNamespace(org, repo, name string) string {
	r := strings.NewReplacer("/", "-", "_", "-")
	base := r.Replace(org) + "-" + r.Replace(repo)
	if name == "" {
		return base
	}
	return base + "--" + r.Replace(name)
}

func operationLocation(org, repo, name, opID string) string {
	return "/api/repos/" + org + "/" + repo + "/apps/" + name + "/operations/" + opID
}

// DB statuses "pending"/"running" map to API "pending"; "succeeded" maps to "complete".
func dbOperationToOAS(op dbqueries.Operation) server.Operation {
	status := server.OperationStatusPending
	switch op.Status {
	case "succeeded":
		status = server.OperationStatusComplete
	case "failed":
		status = server.OperationStatusFailed
	}

	out := server.Operation{
		ID:        op.ID,
		Type:      op.Kind,
		Status:    status,
		Progress:  progressMessage(op),
		CreatedAt: op.CreatedAt,
	}

	if status != server.OperationStatusPending {
		out.CompletedAt = server.OptNilDateTime{Value: op.UpdatedAt, Set: true}
	}

	if op.Error.Valid && status == server.OperationStatusFailed {
		out.Error = server.OptNilErrorDetail{
			Value: server.ErrorDetail{
				Code:    "deploy_failed",
				Message: op.Error.String,
				Remedy:  "check app logs and redeploy",
			},
			Set: true,
		}
	}

	return out
}

func progressMessage(op dbqueries.Operation) string {
	switch op.Status {
	case "pending":
		return "operation queued"
	case "running":
		return "operation in progress"
	case "succeeded":
		return "operation complete"
	case "failed":
		return "operation failed"
	default:
		return op.Status
	}
}

func dbAppToOAS(app dbqueries.App) server.App {
	out := server.App{
		Type:      server.AppType(app.Type),
		Status:    app.Status,
		CreatedAt: server.NewOptDateTime(app.CreatedAt),
		UpdatedAt: server.NewOptDateTime(app.UpdatedAt),
	}
	if app.Name != "" {
		out.Name = server.NewOptString(app.Name)
	}
	if app.Namespace.Valid {
		out.Namespace = server.NewOptString(app.Namespace.String)
	}
	if app.ImageCurrent.Valid {
		out.Image = server.NewOptString(app.ImageCurrent.String)
	}
	return out
}

func dbRepoToOAS(repo dbqueries.Repo, appCount int64) server.Repo {
	return server.Repo{
		Slug:      repo.Slug,
		Tier:      repo.Tier,
		AppCount:  server.NewOptInt(int(appCount)),
		CreatedAt: server.NewOptDateTime(repo.CreatedAt),
	}
}

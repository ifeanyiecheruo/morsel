package handler

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
)

// newOperationID returns a random "op_<hex>" string suitable for use as an operation ID.
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

// operationLocation builds the URL path for polling an operation.
func operationLocation(org, repo, name, opID string) string {
	return "/api/repos/" + org + "/" + repo + "/apps/" + name + "/operations/" + opID
}

// dbOperationToOAS converts a DB operation row to the OAS Operation type.
// DB statuses "pending"/"running" map to API "pending"; "succeeded" maps to "complete".
func dbOperationToOAS(op dbqueries.Operation) oas.Operation {
	status := oas.OperationStatusPending
	switch op.Status {
	case "succeeded":
		status = oas.OperationStatusComplete
	case "failed":
		status = oas.OperationStatusFailed
	}

	out := oas.Operation{
		ID:        op.ID,
		Type:      op.Kind,
		Status:    status,
		Progress:  progressMessage(op),
		CreatedAt: op.CreatedAt,
	}

	if status != oas.OperationStatusPending {
		out.CompletedAt = oas.OptNilDateTime{Value: op.UpdatedAt, Set: true}
	}

	if op.Error.Valid && status == oas.OperationStatusFailed {
		out.Error = oas.OptNilErrorDetail{
			Value: oas.ErrorDetail{
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

// dbAppToOAS converts a DB app row to the OAS App type.
func dbAppToOAS(app dbqueries.App) oas.App {
	out := oas.App{
		Type:      oas.AppType(app.Type),
		Status:    app.Status,
		CreatedAt: oas.NewOptDateTime(app.CreatedAt),
		UpdatedAt: oas.NewOptDateTime(app.UpdatedAt),
	}
	if app.Name != "" {
		out.Name = oas.NewOptString(app.Name)
	}
	if app.Namespace.Valid {
		out.Namespace = oas.NewOptString(app.Namespace.String)
	}
	if app.ImageCurrent.Valid {
		out.Image = oas.NewOptString(app.ImageCurrent.String)
	}
	return out
}

// dbRepoToOAS converts a DB repo row to the OAS Repo type with an app count.
func dbRepoToOAS(repo dbqueries.Repo, appCount int64) oas.Repo {
	return oas.Repo{
		Slug:      repo.Slug,
		Tier:      repo.Tier,
		AppCount:  oas.NewOptInt(int(appCount)),
		CreatedAt: oas.NewOptDateTime(repo.CreatedAt),
	}
}

// retryAfterDeploy is the recommended poll interval for deploy operations.
const retryAfterDeploy = 5 * time.Second

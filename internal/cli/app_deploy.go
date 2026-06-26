package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	"github.com/ifeanyiecheruo/morsel/internal/apiclient"
	"github.com/ifeanyiecheruo/morsel/internal/container"
)

func (c *cli) appDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Build and deploy all apps declared in .morsel/*.morsel.json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.AppDeploy(cmd.Context(), prof)
		},
	}
	return cmd
}

func (h *cliHandler) AppDeploy(ctx context.Context, prof *Profile) error {
	// Authenticated client for the preparation call (uses stored operator token).
	authClient, err := h.clientFor(prof)
	if err != nil {
		return err
	}

	// Derive org/repo: try git remote origin first, fall back to local directory name.
	org, repo, err := deployOrgRepo()
	if err != nil {
		return err
	}

	// Single call to prepare the deploy: issues a repo-scoped deploy token and
	// returns the registry URL (and optional registry credentials).
	res, err := authClient.Inner().PrepareRepoDeploy(ctx, oas.PrepareRepoDeployParams{Org: org, Repo: repo})
	if err != nil {
		return fmt.Errorf("prepare deploy: %w", err)
	}
	session, ok := res.(*oas.DeployConfig)
	if !ok {
		return fmt.Errorf("prepare deploy: unexpected response type %T", res)
	}

	// Build a deploy client scoped to this repo using the returned deploy token.
	deployClient, err := apiclient.New(prof.APIURL, session.Token)
	if err != nil {
		return fmt.Errorf("build deploy client: %w", err)
	}

	apps, err := discoverApps()
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return fmt.Errorf("no .morsel.json files found in .morsel/ — create one before deploying")
	}

	sha := gitCurrentSHA()
	shortSHA := sha
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}

	type deployResult struct {
		name    string
		image   string
		elapsed time.Duration
		err     error
	}
	results := make([]deployResult, len(apps))

	var wg sync.WaitGroup
	for i, app := range apps {
		wg.Add(1)
		go func(idx int, cfg morselConfig) {
			defer wg.Done()
			start := time.Now()
			r := deployResult{name: cfg.Name}
			r.image, r.err = buildPushDeploy(ctx, deployClient, org, repo, cfg, session.Registry, shortSHA)
			r.elapsed = time.Since(start)
			results[idx] = r
		}(i, app)
	}
	wg.Wait()

	var failCount int
	for _, r := range results {
		label := r.name
		if label == "" {
			label = repo
		}
		if r.err != nil {
			failCount++
			fmt.Printf("  ✗ %-20s deploy failed — %v\n", label, r.err)
		} else {
			fmt.Printf("  ✓ %-20s deployed %s  (%s)\n", label, r.image, r.elapsed.Round(time.Second))
		}
	}

	if failCount > 0 {
		return fmt.Errorf("%d app(s) failed to deploy", failCount)
	}
	return nil
}

// deployOrgRepo returns the org and repo for the deploy target.
// Tries git remote origin first; falls back to the local directory name.
func deployOrgRepo() (string, string, error) {
	if org, repo, err := gitRemoteOrgRepo(); err == nil {
		return org, repo, nil
	}
	return localOrgRepo()
}

// buildPushDeploy builds the container image, pushes it to the registry,
// and calls the deploy API for a single app. Returns the pushed image reference.
func buildPushDeploy(ctx context.Context, client *apiclient.Client, org, repo string, cfg morselConfig, registry, shortSHA string) (string, error) {
	dockerfile := cfg.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	pushRef := registry + "/" + appImagePath(repo, cfg.Name, shortSHA)

	if err := buildAndPush(ctx, dockerfile, pushRef, filepath.Dir(dockerfile)); err != nil {
		return "", err
	}

	apiType, err := morselTypeToAPI(cfg.Type)
	if err != nil {
		return "", err
	}
	spec := oas.AppSpec{
		Type:  oas.AppSpecType(apiType),
		Image: pushRef,
	}
	if cfg.Name != "" {
		spec.Name = oas.NewOptString(cfg.Name)
	}
	if cfg.Schedule != "" {
		spec.Schedule = oas.NewOptString(cfg.Schedule)
	}
	if len(cfg.Env) > 0 {
		spec.Env = oas.NewOptAppSpecEnv(oas.AppSpecEnv(cfg.Env))
	}

	deployRes, err := client.Inner().UpsertApp(ctx, &spec, oas.UpsertAppParams{Org: org, Repo: repo})
	if err != nil {
		return "", fmt.Errorf("deploy app: %w", err)
	}
	headers, ok := deployRes.(*oas.AcceptedOperationHeaders)
	if !ok {
		return "", fmt.Errorf("deploy: unexpected response type %T", deployRes)
	}

	if err := pollOperation(ctx, client, org, repo, cfg.Name, headers.Response.OperationID); err != nil {
		return "", err
	}
	return pushRef, nil
}

// appImagePath builds the registry-relative image path for an app.
func appImagePath(repo, appName, shortSHA string) string {
	path := repo
	if appName != "" {
		path += "/" + appName
	}
	if shortSHA != "" {
		path += ":" + shortSHA
	}
	return path
}

// pollOperation polls an operation until it reaches a terminal state.
func pollOperation(ctx context.Context, client *apiclient.Client, org, repo, name, opID string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
		res, err := client.Inner().GetOperation(ctx, oas.GetOperationParams{
			Org: org, Repo: repo, Name: name, ID: opID,
		})
		if err != nil {
			return fmt.Errorf("poll operation: %w", err)
		}
		op, ok := res.(*oas.Operation)
		if !ok {
			return fmt.Errorf("unexpected poll response: %T", res)
		}
		switch op.Status {
		case oas.OperationStatusComplete:
			return nil
		case oas.OperationStatusFailed:
			if op.Error.Set {
				return fmt.Errorf("%s", op.Error.Value.Message)
			}
			return fmt.Errorf("operation failed")
		}
	}
}

// morselConfig is the per-app config read from .morsel/{name}.morsel.json.
type morselConfig struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Dockerfile string            `json:"dockerfile"`
	Schedule   string            `json:"schedule"`
	Env        map[string]string `json:"env"`
}

// discoverApps scans .morsel/*.morsel.json and returns all declared apps.
func discoverApps() ([]morselConfig, error) {
	entries, err := filepath.Glob(".morsel/*.morsel.json")
	if err != nil {
		return nil, fmt.Errorf("scan .morsel/: %w", err)
	}
	apps := make([]morselConfig, 0, len(entries))
	for _, path := range entries {
		cfg, err := readMorselConfigFromPath(path)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			apps = append(apps, *cfg)
		}
	}
	return apps, nil
}

func readMorselConfigFromPath(path string) (*morselConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg morselConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, nil
}

func buildAndPush(ctx context.Context, dockerfilePath, tag, buildContext string) error {
	rt, err := container.CreateRuntime()
	if err != nil {
		return err
	}
	build := exec.CommandContext(ctx, rt.Name(), "build", "-t", tag, "-f", dockerfilePath, buildContext)
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("%s build: %w", rt.Name(), err)
	}
	return rt.Push(ctx, tag)
}

// morselTypeToAPI maps .morsel.json type values to the API AppSpecType string.
// "cronjob" is accepted as an alias for "cron".
func morselTypeToAPI(t string) (string, error) {
	switch t {
	case "http", "worker", "cron":
		return t, nil
	case "cronjob":
		return "cron", nil
	default:
		return "", fmt.Errorf("unknown app type %q (valid: http, worker, cronjob)", t)
	}
}

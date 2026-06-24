package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	"github.com/ifeanyiecheruo/morsel/internal/apiclient"
)

func (c *cli) appDeployCmd(org, repo *string) *cobra.Command {
	var image string

	cmd := &cobra.Command{
		Use:   "deploy NAME",
		Short: "Deploy an app by name, reading config from .morsel/NAME.morsel.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			name := args[0]
			resolvedOrg, resolvedRepo, err := resolveOrgRepo(*org, *repo)
			if err != nil {
				return err
			}

			cfg, err := readMorselConfig(name)
			if err != nil {
				return err
			}
			if cfg == nil {
				return fmt.Errorf(".morsel/%s.morsel.json not found — create it before deploying", name)
			}
			apiType, err := morselTypeToAPI(cfg.Type)
			if err != nil {
				return err
			}

			return c.handler.AppDeploy(cmd.Context(), prof, resolvedOrg, resolvedRepo, name, image, apiType)
		},
	}
	cmd.Flags().StringVar(&image, "image", "", "container image to deploy (e.g. ghcr.io/org/app:v1.0.0)")
	if err := cmd.MarkFlagRequired("image"); err != nil {
		panic(err)
	}
	return cmd
}

func (h *cliHandler) AppDeploy(ctx context.Context, prof *Profile, org, repo, name, image, appType string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}

	spec := oas.AppSpec{
		Name:  oas.NewOptString(name),
		Type:  oas.AppSpecType(appType),
		Image: image,
	}
	res, err := client.Inner().UpsertApp(ctx, &spec, oas.UpsertAppParams{Org: org, Repo: repo})
	if err != nil {
		return fmt.Errorf("deploy app: %w", err)
	}
	headers, ok := res.(*oas.AcceptedOperationHeaders)
	if !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}

	opID := headers.Response.OperationID
	fmt.Printf("Deploying %s/%s/%s (operation %s)...\n", org, repo, name, opID)
	return h.waitForOperation(ctx, client, org, repo, name, opID)
}

// waitForOperation polls an operation until it reaches a terminal state,
// printing progress on each poll.
func (h *cliHandler) waitForOperation(ctx context.Context, client *apiclient.Client, org, repo, name, opID string) error {
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
		fmt.Printf("  %s\n", op.Progress)
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

// morselConfig is the minimal subset of .morsel.json used by the deploy command.
type morselConfig struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// readMorselConfig reads .morsel/{name}.morsel.json; returns nil if the file
// does not exist.
func readMorselConfig(name string) (*morselConfig, error) {
	path := ".morsel/" + name + ".morsel.json"
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

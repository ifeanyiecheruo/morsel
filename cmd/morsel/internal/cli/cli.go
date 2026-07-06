package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/version"
)

// Execute runs the morsel CLI with the production handler against os.Args.
// dockerfile is the embedded Dockerfile for the morsel-api image, used by
// the `service dockerfile` subcommand.
func Execute(ctx context.Context, dockerfile []byte) error {
	return run(ctx, &cliHandler{}, os.Args[1:], dockerfile)
}

type cli struct {
	profileName string
	profile     *Profile
	handler     Handler
	dockerfile  []byte
	levelVar    *slog.LevelVar
}

// run builds and executes the command tree. Tests call this directly with a mock handler.
func run(ctx context.Context, handler Handler, args []string, dockerfile []byte) error {
	ctx, levelVar := ctxlog.Init(ctx)
	c := &cli{handler: handler, dockerfile: dockerfile, levelVar: levelVar}
	root := c.buildRoot()
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

func (c *cli) buildRoot() *cobra.Command {
	var logLevelFlag string
	cmd := &cobra.Command{
		Use:           "morsel",
		Short:         "morsel — self-hosted PaaS for non-production applications",
		Version:       version.Get().String(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&c.profileName, "profile", "default", "profile name")
	cmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "info", "log verbosity: info, trace, or debug")

	loadProfile := c.loadProfilePreRun
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		mode, err := ctxlog.ParseMode(logLevelFlag)
		if err != nil {
			return err
		}
		if c.levelVar != nil {
			c.levelVar.Set(mode.SlogLevel())
		}
		cmd.SetContext(ctxlog.WithMode(cmd.Context(), mode))
		return loadProfile(cmd, args)
	}

	cmd.AddCommand(c.serviceCmd())
	cmd.AddCommand(c.operatorCmd())
	cmd.AddCommand(c.appCmd())
	cmd.AddCommand(c.lintCmd())
	cmd.AddCommand(c.versionCmd())

	return cmd
}

// loadProfilePreRun loads the saved profile and silently refreshes an expired
// access token. Commands that require auth check c.profile via requireProfile().
func (c *cli) loadProfilePreRun(cmd *cobra.Command, _ []string) error {
	prof, err := c.handler.LoadProfile(cmd.Context(), c.profileName, true)
	if err != nil || prof == nil {
		return nil
	}
	c.profile = prof
	return nil
}

func (c *cli) requireProfile() (*Profile, error) {
	if c.profile == nil {
		return nil, fmt.Errorf("not authenticated — run 'morsel operator login' first")
	}
	return c.profile, nil
}

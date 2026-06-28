package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
}

// run builds and executes the command tree. Tests call this directly with a mock handler.
func run(ctx context.Context, handler Handler, args []string, dockerfile []byte) error {
	c := &cli{handler: handler, dockerfile: dockerfile}
	root := c.buildRoot()
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

func (c *cli) buildRoot() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "morsel",
		Short:         "morsel — self-hosted PaaS for non-production applications",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&c.profileName, "profile", "default", "profile name")
	cmd.PersistentPreRunE = c.loadProfilePreRun

	cmd.AddCommand(c.serviceCmd())
	cmd.AddCommand(c.operatorCmd())
	cmd.AddCommand(c.appCmd())
	cmd.AddCommand(c.lintCmd())

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

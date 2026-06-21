package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/platforms"
	"github.com/spf13/cobra"
)

// Execute runs the morsel CLI with the production handler against os.Args.
func Execute(ctx context.Context) error {
	return run(ctx, &cliHandler{}, os.Args[1:])
}

type cli struct {
	profileName string
	profile     *Profile
	platform    platform.Platform // created once when the profile is loaded
	handler     Handler
}

// run builds and executes the command tree. Tests call this directly with a mock handler.
func run(ctx context.Context, handler Handler, args []string) error {
	c := &cli{handler: handler}
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

func (c *cli) loadProfilePreRun(cmd *cobra.Command, _ []string) error {
	prof, err := c.handler.LoadProfile(cmd.Context(), c.profileName, true)
	if err != nil || prof == nil {
		return nil
	}
	c.profile = prof
	// Create the platform instance once so all commands in this invocation share it.
	// Errors here are non-fatal; individual commands fail with a clearer message if
	// they actually try to use the platform.
	if prof.Platform != "" {
		if p, err := platforms.Create(prof.Platform); err == nil {
			c.platform = p
		}
	}
	return nil
}

func (c *cli) requireProfile() (*Profile, error) {
	if c.profile == nil {
		return nil, fmt.Errorf("not authenticated — run 'morsel operator login' first")
	}
	return c.profile, nil
}

func (c *cli) requirePlatform() (platform.Platform, error) {
	if c.platform == nil {
		if c.profile != nil && c.profile.Platform != "" {
			return nil, fmt.Errorf("unsupported platform %q", c.profile.Platform)
		}
		return nil, fmt.Errorf("not authenticated — run 'morsel operator login' first")
	}
	return c.platform, nil
}

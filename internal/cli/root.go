package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Execute runs the morsel CLI with the production handler against os.Args.
func Execute() error {
	return run(&cliHandler{}, os.Args[1:], nil)
}

type cli struct {
	profileName string
	profile     *Profile
	handler     Handler
}

// run builds and executes the command tree. Tests call this directly with a mock
// handler and optional pre-set profile (avoids profile file I/O in tests).
func run(handler Handler, args []string, prof *Profile) error {
	c := &cli{handler: handler, profile: prof}
	root := c.buildRoot()
	root.SetArgs(args)
	return root.Execute()
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

func (c *cli) loadProfilePreRun(_ *cobra.Command, _ []string) error {
	if c.profile != nil {
		return nil // pre-set by run() caller (e.g., tests); skip file load
	}
	prof, err := readProfile(c.profileName)
	if err == nil {
		c.profile = prof
	}
	return nil
}

func (c *cli) requireProfile() (*Profile, error) {
	if c.profile == nil {
		return nil, fmt.Errorf("not authenticated — run 'morsel operator login' first")
	}
	return c.profile, nil
}

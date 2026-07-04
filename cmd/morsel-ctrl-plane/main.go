package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

func main() {
	ctx, levelVar := ctxlog.Init(context.Background())

	var logLevelFlag string
	// TODO: add version support and build info support
	// embedd version info from version file in repo
	// embed build info from file generated during build
	// will need bump-version make target to update version file
	// expose version info via a `version` command and `--version` flag
	// expose build info in help output
	// do this for all cli commands
	root := &cobra.Command{
		Use:           "morsel-ctrl-plane",
		Short:         "Morsel control plane",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(&logLevelFlag, "log-level", "info", "log verbosity: info, trace, or debug")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		mode, err := ctxlog.ParseMode(logLevelFlag)
		if err != nil {
			return err
		}
		levelVar.Set(mode.SlogLevel())
		return nil
	}

	run := &cobra.Command{
		Use:   "run",
		Short: "Run a control-plane service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("specify a service: api, queue, blob, wake-proxy, admin-ui")
		},
	}
	run.AddCommand(
		newAPICmd(ctx),
		newQueueCmd(ctx),
		newBlobCmd(ctx),
		newWakeProxyCmd(ctx),
		newAdminUICmd(ctx),
	)
	root.AddCommand(run)

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

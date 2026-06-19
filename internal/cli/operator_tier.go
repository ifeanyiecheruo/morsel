package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) operatorTierCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tier",
		Short: "Manage quota tiers",
	}
	cmd.AddCommand(c.operatorTierListCmd())
	cmd.AddCommand(c.operatorTierCreateCmd())
	cmd.AddCommand(c.operatorTierEditCmd())
	cmd.AddCommand(c.operatorTierSetDefaultCmd())
	cmd.AddCommand(c.operatorTierDeleteCmd())
	return cmd
}

func (c *cli) operatorTierListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured quota tiers",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierList(prof)
		},
	}
}

func (c *cli) operatorTierCreateCmd() *cobra.Command {
	var flags TierFlags

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new quota tier",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierCreate(prof, flags)
		},
	}
	registerTierFlags(cmd, &flags)
	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorTierEditCmd() *cobra.Command {
	var flags TierFlags

	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit limits on an existing tier",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierEdit(prof, flags)
		},
	}
	registerTierFlags(cmd, &flags)
	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorTierSetDefaultCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "set-default",
		Short: "Set the platform default tier for new repos",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierSetDefault(prof, name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tier name")
	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}
	return cmd
}

func (c *cli) operatorTierDeleteCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a tier",
		RunE: func(_ *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierDelete(prof, name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tier name")
	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}
	return cmd
}

func (h *cliHandler) TierList(_ *Profile) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) TierCreate(_ *Profile, _ TierFlags) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) TierEdit(_ *Profile, _ TierFlags) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) TierSetDefault(_ *Profile, _ string) error {
	return fmt.Errorf("not yet implemented")
}

func (h *cliHandler) TierDelete(_ *Profile, _ string) error {
	return fmt.Errorf("not yet implemented")
}

// TierFlags holds the quota parameters shared by tier create and tier edit.
type TierFlags struct {
	Name           string
	MaxApps        int
	CPU            float64
	MemoryMB       int
	BlobGB         int
	DatabaseGB     int
	QueuesGB       int
	HibernateAfter string
}

func registerTierFlags(cmd *cobra.Command, flags *TierFlags) {
	cmd.Flags().StringVar(&flags.Name, "name", "", "tier name")
	cmd.Flags().IntVar(&flags.MaxApps, "max-apps", 0, "maximum number of apps")
	cmd.Flags().Float64Var(&flags.CPU, "cpu", 0, "CPU cores per app")
	cmd.Flags().IntVar(&flags.MemoryMB, "memory", 0, "memory limit per app in MB")
	cmd.Flags().IntVar(&flags.BlobGB, "blob", 0, "blob storage quota in GB")
	cmd.Flags().IntVar(&flags.DatabaseGB, "database", 0, "database storage quota in GB")
	cmd.Flags().IntVar(&flags.QueuesGB, "queues", 0, "queue storage quota in GB")
	cmd.Flags().StringVar(&flags.HibernateAfter, "hibernate-after", "", "idle duration before hibernation (e.g. 1h)")
}

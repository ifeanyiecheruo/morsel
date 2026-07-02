package cli

import (
	"context"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client/oas"
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierList(cmd.Context(), prof)
		},
	}
}

func (c *cli) operatorTierCreateCmd() *cobra.Command {
	var flags TierFlags

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new quota tier",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierCreate(cmd.Context(), prof, flags)
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierEdit(cmd.Context(), prof, flags)
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierSetDefault(cmd.Context(), prof, name)
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			prof, err := c.requireProfile()
			if err != nil {
				return err
			}
			return c.handler.TierDelete(cmd.Context(), prof, name)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tier name")
	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}
	return cmd
}

func (h *cliHandler) TierList(ctx context.Context, prof *Profile) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().ListTiers(ctx)
	if err != nil {
		return fmt.Errorf("list tiers: %w", err)
	}
	tiers, ok := res.(*oas.ListTiersOKApplicationJSON)
	if !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	if len(*tiers) == 0 {
		fmt.Println("  (no tiers configured)")
		return nil
	}
	fmt.Printf("%-12s  %8s  %6s  %8s  %6s  %8s  %7s  %11s  %s\n",
		"NAME", "MAX-APPS", "CPU", "MEMORY", "BLOB", "DATABASE", "QUEUES", "IDLE-AFTER", "DEFAULT")
	for _, t := range *tiers {
		def := ""
		if t.IsDefault {
			def = "*"
		}
		fmt.Printf("%-12s  %8d  %5dm  %6dMB  %4dGB  %6dGB  %5dGB  %11s  %s\n",
			t.Name, t.MaxApps, t.CPUMilli, t.MemoryMB, t.BlobGB, t.DatabaseGB, t.QueueGB, t.HibernateAfter, def)
	}
	return nil
}

func (h *cliHandler) TierCreate(ctx context.Context, prof *Profile, flags TierFlags) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().CreateTier(ctx, &oas.CreateTierReq{
		Name:           flags.Name,
		MaxApps:        flags.MaxApps,
		CPUMilli:       int(flags.CPU * 1000),
		MemoryMB:       flags.MemoryMB,
		BlobGB:         flags.BlobGB,
		DatabaseGB:     flags.DatabaseGB,
		QueueGB:        flags.QueuesGB,
		HibernateAfter: flags.HibernateAfter,
	})
	if err != nil {
		return fmt.Errorf("create tier: %w", err)
	}
	if _, ok := res.(*oas.Tier); !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	fmt.Printf("✓ Tier %q created.\n", flags.Name)
	return nil
}

func (h *cliHandler) TierEdit(ctx context.Context, prof *Profile, flags TierFlags) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	req := &oas.UpdateTierReq{}
	if flags.MaxApps != 0 {
		req.SetMaxApps(oas.NewOptInt(flags.MaxApps))
	}
	if flags.CPU != 0 {
		req.SetCPUMilli(oas.NewOptInt(int(flags.CPU * 1000)))
	}
	if flags.MemoryMB != 0 {
		req.SetMemoryMB(oas.NewOptInt(flags.MemoryMB))
	}
	if flags.BlobGB != 0 {
		req.SetBlobGB(oas.NewOptInt(flags.BlobGB))
	}
	if flags.DatabaseGB != 0 {
		req.SetDatabaseGB(oas.NewOptInt(flags.DatabaseGB))
	}
	if flags.QueuesGB != 0 {
		req.SetQueueGB(oas.NewOptInt(flags.QueuesGB))
	}
	if flags.HibernateAfter != "" {
		req.SetHibernateAfter(oas.NewOptString(flags.HibernateAfter))
	}
	res, err := client.Inner().UpdateTier(ctx, req, oas.UpdateTierParams{Name: flags.Name})
	if err != nil {
		return fmt.Errorf("edit tier: %w", err)
	}
	if _, ok := res.(*oas.Tier); !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	fmt.Printf("✓ Tier %q updated.\n", flags.Name)
	return nil
}

func (h *cliHandler) TierSetDefault(ctx context.Context, prof *Profile, name string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().SetDefaultTier(ctx, oas.SetDefaultTierParams{Name: name})
	if err != nil {
		return fmt.Errorf("set default tier: %w", err)
	}
	if _, ok := res.(*oas.Tier); !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	fmt.Printf("✓ Default tier set to %q.\n", name)
	return nil
}

func (h *cliHandler) TierDelete(ctx context.Context, prof *Profile, name string) error {
	client, err := h.clientFor(prof)
	if err != nil {
		return err
	}
	res, err := client.Inner().DeleteTier(ctx, oas.DeleteTierParams{Name: name})
	if err != nil {
		return fmt.Errorf("delete tier: %w", err)
	}
	if _, ok := res.(*oas.DeleteTierNoContent); !ok {
		return fmt.Errorf("unexpected response: %T", res)
	}
	fmt.Printf("✓ Tier %q deleted.\n", name)
	return nil
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

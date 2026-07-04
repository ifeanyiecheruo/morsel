package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newBlobCmd(_ context.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "blob",
		Short: "Run the blob storage service (not yet implemented)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("blob service is not yet implemented")
		},
	}
}

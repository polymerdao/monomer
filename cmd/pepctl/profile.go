package main

import (
	"github.com/spf13/cobra"
)

func profileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Profile peptide components",
	}
	cmd.AddCommand(profileStoreRollbackCmd())
	return cmd
}

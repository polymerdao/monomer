package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/polymerdao/monomer/monogen"
	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "monogen",
		Short: "monogen scaffolds a Monomer project.",
		Long: "monogen scaffolds a Monomer project. " +
			"The resulting project is compatible with the ignite tool (https://github.com/ignite/cli).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return monogen.Generate(cmd.Context(), appDirPath, goModulePath, addressPrefix, skipGit)
		},
	}

	skipGit       bool
	appDirPath    string
	goModulePath  string
	addressPrefix string
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	rootCmd.Flags().BoolVar(&skipGit, "skip-git", false, "skip git repository initialization")
	rootCmd.Flags().StringVar(&appDirPath, "app-dir-path", "./testapp", "project directory")
	rootCmd.Flags().StringVar(&goModulePath, "gomod-path", "github.com/testapp/testapp", "go module path")
	rootCmd.Flags().StringVar(&addressPrefix, "address-prefix", "cosmos", "address prefix")

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v", err)
		cancel()   // cancel is not called on os.Exit, we have to call it manually
		os.Exit(1) //nolint:gocritic // Doesn't recognize that cancel() is called.
	}
}

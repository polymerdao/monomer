package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/polymerdao/monomer/environment"
)

func Run(
	ctx context.Context,
	env *environment.Env,
	outDir string,
) error {
	monomerPath, err := filepath.Abs("..")
	if err != nil {
		return fmt.Errorf("absolute path to local monomer directory: %v", err)
	}
	const appName = "e2eapp"
	appDirPath := filepath.Join(outDir, appName)
	monogenCmd := setupCmd(exec.CommandContext(ctx,
		filepath.Join("..", "scripts", "go-wrapper.sh"),
		"run", filepath.Join("..", "cmd", "monogen"),
		"--app-dir-path", appDirPath,
		"--gomod-path", "github.com/e2e/"+appName,
		"--address-prefix", "e2e",
		"--monomer-path", monomerPath,
	))
	if err := monogenCmd.Run(); err != nil {
		return fmt.Errorf("run monogen: %v", err)
	}

	setupHelperCmd := setupCmd(exec.CommandContext(ctx, filepath.Join(appDirPath, "setup-helper.sh")))
	setupHelperCmd.Dir = appDirPath
	setupHelperCmd.Env = append(os.Environ(), "e2eapp_HOME="+outDir)
	if err := setupHelperCmd.Run(); err != nil {
		return fmt.Errorf("run setup helper: %v", err)
	}

	appCmd := setupCmd(exec.CommandContext(ctx,
		filepath.Join(appDirPath, appName+"d"),
		"monomer",
		"start",
		"--minimum-gas-prices", "0.001ETH",
		"--monomer.dev-start",
	))
	appCmd.Dir = appDirPath
	appCmd.Env = append(os.Environ(), "e2eapp_HOME="+outDir)
	if err := appCmd.Start(); err != nil {
		return fmt.Errorf("run app: %v", err)
	}
	env.DeferErr("wait for app", appCmd.Wait)

	return nil
}

func setupCmd(cmd *exec.Cmd) *exec.Cmd {
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

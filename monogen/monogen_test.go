package monogen_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	bftclient "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/monogen"
	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	const appName = "monomerapp"
	rootDirPath := t.TempDir()
	appDirPath := filepath.Join(rootDirPath, appName)
	pwd, err := os.Getwd()
	require.NoError(t, err)
	monomerPath := filepath.Clean(filepath.Join("..", pwd))

	goModPath := "github.com/test/" + appName
	const addressPrefix = "test"

	// Generate project.
	require.NoError(t, monogen.Generate(context.Background(), appDirPath, goModPath, addressPrefix, monomerPath))

	// Run monogen.sh.
	require.NoError(t, initCommand(t, exec.Command(filepath.Join(appDirPath, "monogen.sh")), appName, rootDirPath, appDirPath).Run())

	testApp(t, rootDirPath, appDirPath, appName)

	// Cannot overwrite existing directory.
	require.ErrorContains(t, monogen.Generate(context.Background(), appDirPath, goModPath, addressPrefix, monomerPath), "refusing to overwrite")
}

func initCommand(t *testing.T, cmd *exec.Cmd, appName, rootDirPath, appDirPath string) *exec.Cmd {
	cmd.Dir = appDirPath
	// Set a different home directory to avoid cluttering the home directory of the person running the script.
	// Ignite prefixes environment variables with the lowercase app name instead of uppercase for some reason.
	cmd.Env = append(os.Environ(), appName+"_HOME="+rootDirPath)
	logger := logWriter(t.Log)
	cmd.Stdout = logger
	cmd.Stderr = logger
	return cmd
}

func testApp(t *testing.T, rootDirPath, appDirPath, appName string) {
	// Start the app.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := initCommand(
		t,
		exec.CommandContext(ctx, filepath.Join(appDirPath, appName+"d"), "monomer", "start", "--minimum-gas-prices", "0.0001stake"),
		appName,
		rootDirPath,
		appDirPath,
	)
	require.NoError(t, cmd.Start())
	defer func() {
		cancel()
		// TODO: handle this error.
		// It is kind of harmless, just `signal: killed` or `signal: terminated`,
		// but it would be nice to be able to say `require.NoError` here.
		_ = cmd.Wait()
	}()

	// Hit a comet endpoint.
	cometURL, err := url.ParseString("http://127.0.0.1:26657")
	require.NoError(t, err)
	cometURL.IsReachable(context.Background())
	client, err := bftclient.New(cometURL.String(), "/websocket")
	require.NoError(t, err)
	blockNumber := int64(1)
	block, err := client.Block(context.Background(), &blockNumber)
	require.NoError(t, err)
	// Don't worry too much about API correctness, that is tested elsewhere.
	require.Equal(t, blockNumber, block.Block.Height)
}

type logWriter func(...any)

func (lw logWriter) Write(p []byte) (int, error) {
	lw(string(p))
	return len(p), nil
}

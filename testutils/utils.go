package testutils

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"testing"
	"time"

	cometdb "github.com/cometbft/cometbft-db"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"
)

func NewCometMemDB(t *testing.T) cometdb.DB {
	db := cometdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func NewMemDB(t *testing.T) dbm.DB {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

// a timestamp helper
func ts() string {
	return time.Now().Format("[15:04:05.000] ")
}

// LogProcess captures the stdout and stderr of a process and writes it to
// a file.
func LogProcess(name string, cmd *exec.Cmd) error {
	cmdOut, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe stderr: %v", err)
	}
	cmdErr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("pipe stderr: %v", err)
	}

	outScanner := bufio.NewScanner(cmdOut)
	errScanner := bufio.NewScanner(cmdErr)

	now := time.Now().Format("01-02-15h04m05")
	stampedDest := fmt.Sprintf("./artifacts/%s_%s.log", name, now)
	latestDest := fmt.Sprintf("./artifacts/%s_latest.log", name)

	fileMode := fs.FileMode(0o644)
	dirMode := fs.FileMode(0o755)

	err = os.MkdirAll("./artifacts", dirMode)
	if err != nil {
		return fmt.Errorf("create artifacts dir: %v", err)
	}

	stampedFile, err := os.OpenFile(stampedDest, os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open %s out file: %v", name, err)
	}
	latestFile, err := os.OpenFile(latestDest, os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open %s out file: %v", name, err)
	}

	stampedWriter := io.Writer(stampedFile)
	latestWriter := io.Writer(latestFile)

	go func() {
		for outScanner.Scan() {
			line := ts() + outScanner.Text() + "\n"
			if _, err := stampedWriter.Write([]byte(line)); err != nil {
				panic(fmt.Errorf("writing to %s: %v", stampedDest, err))
			}
			if _, err := latestWriter.Write([]byte(line)); err != nil {
				panic(fmt.Errorf("writing to %s: %v", latestDest, err))
			}
		}
	}()
	go func() {
		for errScanner.Scan() {
			line := ts() + "[stderr] " + outScanner.Text() + "\n"
			if _, err := stampedWriter.Write([]byte(line)); err != nil {
				panic(fmt.Errorf("writing to %s: %v", stampedDest, err))
			}
			if _, err := latestWriter.Write([]byte(line)); err != nil {
				panic(fmt.Errorf("writing to %s: %v", latestDest, err))
			}
		}
	}()

	return nil
}

package testutils

import (
	"bufio"
	"fmt"
	"io"
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
	dest := fmt.Sprintf("./artifacts/%s_%s.log", name, now)

	err = os.MkdirAll("./artifacts", 0755)
	if err != nil {
		return fmt.Errorf("create artifacts dir: %v", err)
	}

	destFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open %s out file: %v", name, err)
	}

	destWriter := io.Writer(destFile)

	go func() {
		for outScanner.Scan() {
			line := ts() + outScanner.Text() + "\n"
			if _, err := destWriter.Write([]byte(line)); err != nil {
				panic(fmt.Errorf("writing to %s: %v", dest, err))
			}
		}
	}()
	go func() {
		for errScanner.Scan() {
			line := ts() + "[stderr] " + outScanner.Text() + "\n"
			if _, err := destWriter.Write([]byte(line)); err != nil {
				panic(fmt.Errorf("writing to %s: %v", dest, err))
			}
		}
	}()

	return nil
}

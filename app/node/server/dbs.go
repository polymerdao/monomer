package server

import (
	"log"
	"os"
	"path/filepath"

	tmdb "github.com/cometbft/cometbft-db"
)

func OpenDB(name string, config *Config) tmdb.DB {
	dataDir := filepath.Join(config.HomeDir, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Panicf("could not mkdir, homedir: %s, name: %s, err: %v", config.HomeDir, name, err)
	}
	db, err := tmdb.NewDB(name, config.DbBackend, dataDir)
	if err != nil {
		log.Panicf("could not open db, homedir: %s, name: %s, err: %v", config.HomeDir, name, err)
	}
	return db
}

func OpenDBextended(name string, home string, dbtype DbBackendType) tmdb.DB {
	return OpenDB(name, &Config{
		HomeDir:   home,
		DbBackend: dbtype,
	})
}

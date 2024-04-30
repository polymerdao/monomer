package testutil

import (
	"testing"

	tmdb "github.com/cometbft/cometbft-db"
	"github.com/stretchr/testify/require"
)

func NewMemDB(t *testing.T) tmdb.DB {
	db := tmdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

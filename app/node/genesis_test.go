package node

import (
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
)

type genesisTestSuite struct {
	suite.Suite

	homedir string
	genesis *PeptideGenesis
}

func (s *genesisTestSuite) SetupTest() {
	homedir, err := os.MkdirTemp("", "test-genesis-*")
	s.NoError(err)

	s.homedir = homedir
	s.genesis = &PeptideGenesis{
		GenesisTime: time.Unix(1699473809, 0).UTC(),
		GenesisBlock: eth.BlockID{
			Hash:   common.HexToHash("0x1"),
			Number: 2,
		},
		ChainID:  "123",
		AppState: []byte("state"),
		L1: eth.BlockID{
			Hash:   common.HexToHash("0x3"),
			Number: 4,
		},
		InitialHeight: 5,
	}
}
func (s *genesisTestSuite) TearDownTest() {
	os.RemoveAll(s.homedir)
}

func (s *genesisTestSuite) TestGenesisMarshalUnmarshal() {
	s.NoError(s.genesis.Save(s.homedir, false))
	newGenesis, err := PeptideGenesisFromFile(s.homedir)
	s.NoError(err)
	s.True(reflect.DeepEqual(s.genesis, newGenesis))
}

func (s *genesisTestSuite) TestGenesisSaveOverride() {
	s.NoError(s.genesis.Save(s.homedir, false))
	s.genesis.ChainID = "11"
	s.NoError(s.genesis.Save(s.homedir, true))
	s.Equal(s.genesis.ChainID, lo.Must(PeptideGenesisFromFile(s.homedir)).ChainID)
}

func (s *genesisTestSuite) TestGenesisFromFileValidates() {
	s.genesis.ChainID = "foo"
	s.NoError(s.genesis.Save(s.homedir, false))
	_, err := PeptideGenesisFromFile(s.homedir)
	s.ErrorContains(err, "chain-id must be numerical")
}

func (s *genesisTestSuite) TestValidateFailures() {
	for _, tc := range []struct {
		desc   string
		modify func(g *PeptideGenesis)
		err    string
	}{
		{
			desc:   "valid genesis",
			modify: func(g *PeptideGenesis) {},
		},
		{
			desc:   "invalid chain id",
			modify: func(g *PeptideGenesis) { g.ChainID = "foo" },
			err:    "chain-id must be numerical",
		},
		{
			desc:   "invalid genesis block hash",
			modify: func(g *PeptideGenesis) { g.GenesisBlock.Hash = common.Hash{} },
			err:    "genesis block hash must not be empty",
		},
		{
			desc:   "invalid genesis block height",
			modify: func(g *PeptideGenesis) { g.GenesisBlock.Number = 0 },
			err:    "genesis block height must not be zero",
		},
		{
			desc:   "invalid l1 hash",
			modify: func(g *PeptideGenesis) { g.L1.Hash = common.Hash{} },
			err:    "l1 hash must not be empty",
		},
		{
			desc:   "invalid genesis time",
			modify: func(g *PeptideGenesis) { g.GenesisTime = time.Unix(0, 0) },
			err:    "genesis time must not be empty",
		},
		{
			desc:   "invalid genesis time",
			modify: func(g *PeptideGenesis) { g.AppState = []byte{} },
			err:    "app state must not be empty",
		},
	} {
		s.Run(tc.desc, func() {
			s.SetupTest()
			tc.modify(s.genesis)
			err := s.genesis.Validate()
			if len(tc.err) == 0 {
				s.NoError(err)
			} else {
				s.ErrorContains(err, tc.err)
			}
		})
	}
}

func TestGenesisTestSuite(t *testing.T) {
	suite.Run(t, new(genesisTestSuite))
}

package module_tests

/*
import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	tmdb "github.com/cometbft/cometbft-db"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/polymerdao/monomer/app/node/server"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
)
*/

/*
type MsgServerTestSuite struct {
	suite.Suite
	app *peptide.PeptideApp

	sysConfig    eth.SystemConfig
	rollupConfig *rollup.Config
	l1info       eth.BlockInfo
}

func (s *MsgServerTestSuite) SetupTest() {
	s.app = peptide.New("", "", tmdb.NewMemDB(), server.DefaultLogger())
	stateBytes, err := json.Marshal(s.app.SimpleGenesis(peptest.Accounts, peptest.ValidatorAccounts))
	s.NoError(err)
	lastHeader := s.app.Init(stateBytes, 1, time.Now())
	s.app.Resume(lastHeader, stateBytes)

	rnd := rand.New(rand.NewSource(int64(1234)))
	l1info := testutils.RandomBlockInfo(rnd)
	l1info.InfoNum = 42
	s.l1info = l1info
	s.rollupConfig = &rollup.Config{}

	s.sysConfig = eth.SystemConfig{BatcherAddr: testutils.RandomAddress(rnd), GasLimit: 1234567}
}

func (s *MsgServerTestSuite) TestMsgServer() {

	l1sysTxBytes := lo.Must(derive.L1InfoDepositBytes(s.rollupConfig, s.sysConfig, 0, s.l1info, 0))
	l1userTxBytes := hexutil.MustDecode("0x7ef85da0f68cf7d1d457abc94560ed47a9bd3bd391dd2a01861487f6fefec0407c4edf359415d34aaf54267db7d7c367839aaf71a00a2c6a659415d34aaf54267db7d7c367839aaf71a00a2c6a6585e8d4a5100085e8d4a51000830f42408080")
	msg := types.NewMsgL1Txs([][]byte{l1sysTxBytes, l1userTxBytes})
	result, err := s.app.RunMsgs(s.app.NewUncachedSdkContext(), msg)
	s.Require().NoError(err)
	// more than 1 event
	s.Require().Greater(len(result.Events), 1)
	s.Require().Equal(1, len(result.MsgResponses))

	// assert we can cast 1st response
	_, ok := result.MsgResponses[0].GetCachedValue().(*types.MsgL1TxsResponse)
	s.Require().True(ok)

	// find match mint event
	expectedBalance := hexutil.MustDecode("0xe8d4a51000")
	cosmAddr := "polymer1zhf54t65ye7m047rv7pe4tm35q9zc6n9cs4ank"

	foundEvent := result.Events[0]
	foundBalance := []byte{}

	for _, event := range result.Events {
		if event.Type == types.EventTypeMintETH {
			for _, attr := range event.Attributes {
				if attr.Key == types.AttributeKeyToCosmosAddress && attr.Value == cosmAddr {
					foundEvent = event
					break
				}
			}
		}
	}
	for _, attr := range foundEvent.Attributes {
		if attr.Key == types.AttributeKeyAmount {
			foundBalance = hexutil.MustDecode(attr.Value)
		}
	}
	s.Require().Equal(expectedBalance, foundBalance)
}

func TestMsgServerTestSuite(t *testing.T) {
	suite.Run(t, new(MsgServerTestSuite))
}
*/

package node

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"math/rand"
	"net/http"
	"testing"
	"time"

	tmdb "github.com/cometbft/cometbft-db"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	tmrpc "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/app/node/server"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	peptest "github.com/polymerdao/monomer/testutil/peptide"
	"github.com/polymerdao/monomer/testutil/peptide/eeclient"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
)

type ChainServerTestSuite struct {
	suite.Suite
	app               *peptide.PeptideApp
	client            *eeclient.EeClient
	node              *PeptideNode
	rng               *rand.Rand
	accounts          peptide.SignerAccounts
	validatorAccounts peptide.SignerAccounts
}

func (s *ChainServerTestSuite) SetupTest() {
	logger := server.DefaultLogger()
	appdb := tmdb.NewMemDB()
	bsdb := tmdb.NewMemDB()
	txIndexerDb := tmdb.NewMemDB()
	chainId := "123"
	app := peptide.New(chainId, "", appdb, logger)

	s.accounts = peptest.Accounts
	s.validatorAccounts = peptest.ValidatorAccounts
	stateBytes, err := json.Marshal(app.SimpleGenesis(s.accounts, s.validatorAccounts))
	s.NoError(err)

	rng := rand.New(rand.NewSource(int64(1234)))
	appEndpoint := server.NewEndpoint("localhost:0")
	eeEndpoint := server.NewEndpoint("localhost:0")
	genesis := PeptideGenesis{
		GenesisTime: time.Now(),
		AppState:    stateBytes,
		ChainID:     chainId,
	}

	_, err = InitChain(app, bsdb, &genesis)
	s.NoError(err)

	node := NewPeptideNode(
		bsdb,
		txIndexerDb,
		&appEndpoint,
		&eeEndpoint,
		app,
		NewLocalClient,
		&genesis,
		server.EnableAllApis(),
		logger,
	)

	s.NoError(node.Service().Start())

	cosmosUrl := "http://" + node.CometServerAddress().String()
	eeUrl := "http://" + node.EngineServerAddress().String()
	client, err := eeclient.NewEeClient(context.Background(), cosmosUrl, eeUrl)
	s.NoError(err)

	s.app = app
	s.node = node
	s.client = client
	s.rng = rng
}

func (s *ChainServerTestSuite) TearDownTest() {
	s.client.Close()
	s.node.Service().Stop()
}

func (s *ChainServerTestSuite) TestBlockByNumber() {
	block, err := s.client.BlockByNumber(eth.Unsafe)
	s.NoError(err)
	hash := block.Hash()
	for _, tc := range []struct {
		desc   string
		id     any
		hasErr bool
	}{{
		desc: "safe label",
		id:   eth.Safe,
	}, {
		desc: "finalized label",
		id:   eth.Finalized,
	}, {
		desc: "int",
		id:   1,
	}, {
		desc: "decimal number as a string",
		id:   "1",
	}, {
		desc: "hex number as a string",
		id:   "0x1",
	}, {
		desc: "big int",
		id:   big.NewInt(1),
	}, {
		desc:   "boolean",
		id:     true,
		hasErr: true,
	}, {
		desc:   "invalid label",
		id:     "foo",
		hasErr: true,
	}} {
		s.Run(tc.desc, func() {
			block, err = s.client.BlockByNumber(tc.id)
			if !tc.hasErr {
				s.NoErrorf(err, "error with value %v (%T)", tc.id, tc.id)
				s.Equalf(hash, block.Hash(), "hash mismatch with value %v (%T)", tc.id, tc.id)
			} else {
				s.Nil(err)
				s.Nil(block)
			}
		})
	}
}

func (s *ChainServerTestSuite) TestBlockBySpecialNumber() {
	node := peptest.NewOpNodeMock(&s.Suite, s.client, s.rng)
	node.RunDerivationLoop(5, s.accounts[0], s.accounts[1])
	for _, tc := range []struct {
		desc   string
		number ethrpc.BlockNumber
		label  eth.BlockLabel
	}{{
		desc:   "pending",
		number: ethrpc.PendingBlockNumber,
		label:  eth.Unsafe,
	}, {
		desc:   "latest",
		number: ethrpc.LatestBlockNumber,
		label:  eth.Unsafe,
	}, {
		desc:   "safe",
		number: ethrpc.SafeBlockNumber,
		label:  eth.Safe,
	}, {
		desc:   "finalized",
		number: ethrpc.FinalizedBlockNumber,
		label:  eth.Finalized,
	}} {
		s.Run(tc.desc, func() {
			bynumber, err := s.client.BlockByNumber(tc.number)
			s.NoError(err)
			bybignumber, err := s.client.BlockByNumber(big.NewInt(tc.number.Int64()))
			s.NoError(err)
			bylabel, err := s.client.BlockByNumber(tc.label)
			s.NoError(err)

			s.Equal(bylabel.Hash(), bynumber.Hash())
			s.Equal(bylabel.Hash(), bybignumber.Hash())
		})
	}
}

func (s *ChainServerTestSuite) TestBlockByNilIsLatest() {
	bynil, err := s.client.BlockByNumber(nil)
	s.NoError(err)
	bylabel, err := s.client.BlockByNumber(eth.Unsafe)
	s.NoError(err)

	s.Equal(bylabel.NumberU64(), bynil.NumberU64())
	s.Equal(bylabel.Hash(), bynil.Hash())
}

func (s *ChainServerTestSuite) TestPeptideAndEthBlocks() {
	node := peptest.NewOpNodeMock(&s.Suite, s.client, s.rng)
	for i := 0; i < 5; i++ {
		lastEth, err := s.client.BlockByNumber(eth.Unsafe)
		s.NoError(err)

		lastPeptide, err := s.client.PeptideBlock(eth.Unsafe)
		s.NoError(err)

		s.Equal(lastPeptide.Hash(), lastEth.Hash())
		s.Equal(lastPeptide.Height(), lastEth.Number().Int64())
		s.Equal(lastPeptide.Header.Time, lastEth.Time())
		s.Equal(lastPeptide.ParentHash(), lastEth.ParentHash())
		s.Equal(uint64(lastPeptide.GasLimit), lastEth.GasLimit())

		s.Equal(common.BytesToHash(lastPeptide.Header.AppHash), lastEth.Root())

		txs, txhash := lastPeptide.Transactions()
		ethtxs := lastEth.Transactions()
		s.Equal(len(txs), len(ethtxs))
		s.Equal(txhash, lastEth.TxHash())
		for i, tx := range txs {
			s.Equal(tx.Hash(), ethtxs[i].Hash())
		}
		node.ProduceBlocks(1)
	}
}

func (s *ChainServerTestSuite) TestCommitBlock() {
	s.T().Skip("skip until migrating admin apis")
	lastBlock, err := s.client.BlockByNumber(eth.Unsafe)
	s.NoError(err)
	lastHeight := lastBlock.NumberU64()
	s.Equal(uint64(0), lastHeight)
	// turn engine mode off to disable L1Txs
	s.client.SetEngineMode(false)

	for i := 0; i < 5; i++ {
		// query the last committed block height
		resp, err := s.client.CommitBlock()
		s.NoError(err)

		height := uint64(resp.Response.LastBlockHeight)
		s.Equal(lastHeight+1, height)
		lastHeight = height

		// assert current block's parentHash is the hash of the previous block
		var expectedParentHash eetypes.Hash
		if height == 0 {
			expectedParentHash = eetypes.HashOfEmptyHash
		} else {
			block, err := s.client.BlockByNumber(height - 1)
			s.NoError(err)
			expectedParentHash = block.Hash()
		}
		block, err := s.client.BlockByNumber(height)
		s.NoError(err)
		s.Equal(expectedParentHash, block.ParentHash())
	}
}

func (s *ChainServerTestSuite) TestEngine() {
	node := peptest.NewOpNodeMock(&s.Suite, s.client, s.rng)
	node.RunDerivationLoop(5, s.accounts[0], s.accounts[1])
}

func (s *ChainServerTestSuite) TestEmitEvents() {
	node := peptest.NewOpNodeMock(&s.Suite, s.client, s.rng)
	eventsChan := make(chan []json.RawMessage, 1)
	// TODO: multiple L2 txs should also succeed; but failed txs don't result in events, so we need to fix OpNodeMock.DeliverL2Txs
	numOfL2Txs := 1
	expectedEventsCount := numOfL2Txs + 1 // 1 L1 deposit tx event and numOfTxs L2 events
	go s.collectEvents("tm.event='Tx'", expectedEventsCount, 10*time.Second, eventsChan)

	// create a new payload
	payloadId := node.StartPayload(0, nil)

	// L2 chain gathers L2 txs before block is sealed
	node.DeliverL2Txs(uint64(numOfL2Txs), s.accounts[0], s.accounts[1])

	blockHash := node.ConfirmPayload(payloadId)
	_ = blockHash

	events := <-eventsChan
	s.Equal(expectedEventsCount, len(events))
}

func (s *ChainServerTestSuite) TestL1UserDeposit() {
	node := peptest.NewOpNodeMock(&s.Suite, s.client, s.rng)

	eventsChan := make(chan []json.RawMessage, 1)
	go s.collectEvents("tm.event='Tx'", 1, 10*time.Second, eventsChan)

	userDepositTx := hexutil.MustDecode(
		"0x7ef85da0f68cf7d1d457abc94560ed47a9bd3bd391dd2a01861487f6fefec0407c4" +
			"edf359415d34aaf54267db7d7c367839aaf71a00a2c6a659415d34aaf54267db7d7c367839aaf71" +
			"a00a2c6a6585e8d4a5100085e8d4a51000830f42408080",
	)
	// create a new payload with a L1 user deposit tx
	payloadId := node.StartPayload(0, nil, userDepositTx)
	s.Require().NotNil(payloadId)
	blockHash := node.ConfirmPayload(payloadId)
	node.UpdateL2Head(blockHash, true)

	addr := common.HexToAddress("0x15d34aaf54267db7d7c367839aaf71a00a2c6a65")
	// get cosmos address
	// confirm balance
	balance, err := s.client.GetBalance(addr, eth.Unsafe)
	s.Require().NoError(err)
	expectedBalance := hexutil.MustDecodeBig("0xe8d4a51000")
	s.Equal(expectedBalance, balance)

	events := <-eventsChan

	// all L1 txs are processed in one tx and thus one event is emitted
	s.Equal(1, len(events))
	s.Contains(string(events[0]), "mint_eth.amount")
	s.Contains(string(events[0]), "0xe8d4a51000")
}

// collectEvents collects events from the websocket client and return when one of the following conditions is met:
// 1. the number of events collected is equal to the number of expected events
// 2. the timer expires
func (s *ChainServerTestSuite) collectEvents(
	query string,
	numOfEvents int,
	timeOut time.Duration,
	resultChan chan<- []json.RawMessage,
) {
	wsClient := lo.Must(tmrpc.NewWS("//"+s.node.CometServerAddress().String(), "/websocket"))
	lo.Must0(wsClient.Start())
	defer func() {
		if wsClient.IsActive() {
			wsClient.Stop()
		}
	}()
	wsClient.Subscribe(context.Background(), "tm.event='Tx'")
	timer := time.NewTimer(timeOut)
	events := []json.RawMessage{}

	go func() {
		for {
			select {
			case resp := <-wsClient.ResponsesCh:
				s.T().Logf("Received event with result: %s", string(resp.Result))
				var resultEvent ctypes.ResultEvent
				err := cmtjson.Unmarshal(resp.Result, &resultEvent)
				if err == nil && len(resultEvent.Events) > 0 {
					// ignore empty events
					events = append(events, resp.Result)
				}
				if len(events) == numOfEvents {
					if !timer.Stop() {
						<-timer.C
					}
					timer.Reset(0)
					return
				}
			case <-wsClient.Quit():
				return
			}
		}
	}()

	// either time out or collect enough events
	<-timer.C
	wsClient.Stop()
	resultChan <- events
}

// This test simulates a reorg by calling ForkchoiceUpdate with an "old" block. These are the steps
//
//  1. Produce 10 blocks each with an L2 transaction.
//     Store the blocks and their transactions in an aux list
//  2. Call ForkchoiceUpdate using a block hash from the block list.
//     This will trigger the re-org
//  3. Walk the block and txs aux lists and confirm that all blocks/txs after that used in FCU
//     no longer exist (i.e. they have been removed from peptide)
//  4. Complete the block production with GetPayload/NewPayload
//  5. Check that the newly created block's parent matches with that of the "reorg" and
//     that the heights are the expected ones
func (s *ChainServerTestSuite) TestReorg() {
	node := peptest.NewOpNodeMock(&s.Suite, s.client, s.rng)
	sender := peptest.Accounts[0]
	receiver := peptest.Accounts[1]
	numBlocks := 10
	reorgIndex := 4
	var transactions [][]byte
	var blocks []*ethtypes.Block

	node.ProduceBlocks(1)

	for i := 0; i < numBlocks; i++ {
		tx, err := node.SendTokens(sender, receiver, 1000)
		s.NoError(err)

		// run the loop derivation to include the tx
		node.ProduceBlocks(1)

		// get the latest block and save both for later
		block := node.CurrentBlock()
		transactions = append(transactions, tx.Hash)
		blocks = append(blocks, block)
	}

	reorgBlockHash := blocks[reorgIndex].Hash()
	payloadID := node.StartPayload(0, &reorgBlockHash)

	for i := 0; i < numBlocks; i++ {
		block, berr := node.BlockByNumber(blocks[i].NumberU64())
		tx, terr := node.Tx(transactions[i], false)
		if i > reorgIndex {
			s.Require().Nil(block)
			// non-existent block in BlockByNumber should return (nil, nil)
			s.Require().NoError(berr)

			s.Require().Nil(tx)
			s.Require().Error(terr)
		} else {
			s.Require().NoError(berr)
			s.Require().NoError(terr)

			s.Require().NotNil(block)
			s.Require().NotNil(tx)
		}
	}

	hash := node.ConfirmPayload(payloadID)
	newBlock, err := node.BlockByHash(hash)
	s.NoError(err)

	// Ensure the new block belongs to the new branch
	s.Equal(blocks[reorgIndex].Hash(), newBlock.ParentHash())
	s.Equal(blocks[reorgIndex].NumberU64()+1, newBlock.NumberU64())
	s.Equal(blocks[reorgIndex+1].NumberU64(), newBlock.NumberU64())

	// and finally confirm the new block differs from the previous branch
	s.NotEqual(blocks[reorgIndex+1].Hash(), newBlock.Hash())
}

func (s *ChainServerTestSuite) TestChainId() {
	chainId, err := s.client.ChainID()
	s.Require().NoError(err)
	s.Equal(big.NewInt(123), chainId)
}

func (s *ChainServerTestSuite) TestGenesisReproducibility() {
	// test that creating a simple genesis state doesn't have any side effects
	g := s.app.SimpleGenesis(s.accounts, s.validatorAccounts)
	stateBytes, err := json.Marshal(g)
	s.NoError(err)

	s.Equal(stateBytes, s.node.genesis.AppState)
}

func (s *ChainServerTestSuite) TestMetrics() {
	cosmosUrl := "http://" + s.node.CometServerAddress().String()

	response, err := http.Get(cosmosUrl + "/metrics")
	s.NoError(err)
	defer response.Body.Close()

	s.Equal(http.StatusOK, response.StatusCode, "Expected status code 200, but got %d", response.StatusCode)

	_, err = io.ReadAll(response.Body)
	s.NoError(err)
}

func TestChainServerTestSuite(t *testing.T) {
	suite.Run(t, new(ChainServerTestSuite))
}

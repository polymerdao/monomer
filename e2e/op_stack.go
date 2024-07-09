package e2e

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math"
	"time"

	"github.com/ethereum-optimism/optimism/op-batcher/batcher"
	"github.com/ethereum-optimism/optimism/op-batcher/compressor"
	opbatchermetrics "github.com/ethereum-optimism/optimism/op-batcher/metrics"
	opnodemetrics "github.com/ethereum-optimism/optimism/op-node/metrics"
	opnode "github.com/ethereum-optimism/optimism/op-node/node"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/driver"
	"github.com/ethereum-optimism/optimism/op-node/rollup/sync"
	opproposermetrics "github.com/ethereum-optimism/optimism/op-proposer/metrics"
	"github.com/ethereum-optimism/optimism/op-proposer/proposer"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/crypto"
	"github.com/ethereum-optimism/optimism/op-service/dial"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/utils"
	"golang.org/x/exp/slog"
)

type OPEventListener interface {
	// Log may be called many times.
	Log(r slog.Record)
}

type OPStack struct {
	l1URL               *url.URL
	engineURL           *url.URL
	nodeURL             *url.URL
	privKey             *ecdsa.PrivateKey
	rollupConfig        *rollup.Config
	l2OutputOracleProxy common.Address
	eventListener       OPEventListener
}

// TODO setup verifiers

func NewOPStack(
	l1URL,
	engineURL,
	nodeURL *url.URL,
	l2OutputOracleProxy common.Address,
	privKey *ecdsa.PrivateKey,
	rollupConfig *rollup.Config,
	eventListener OPEventListener,
) *OPStack {
	return &OPStack{
		l1URL:               l1URL,
		engineURL:           engineURL,
		nodeURL:             nodeURL,
		privKey:             privKey,
		rollupConfig:        rollupConfig,
		l2OutputOracleProxy: l2OutputOracleProxy,
		eventListener:       eventListener,
	}
}

func (op *OPStack) Run(ctx context.Context, env *environment.Env) error {
	l1RPCClient, err := rpc.DialContext(ctx, op.l1URL.String())
	if err != nil {
		return fmt.Errorf("dial L1: %v", err)
	}
	l1 := NewL1Client(l1RPCClient)

	if err := op.runNode(ctx, env); err != nil {
		return err
	}

	// Use the same tx manager config for the op-proposer and op-batcher.
	defaults := txmgr.DefaultBatcherFlagValues
	l1ChainID, err := l1.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("get l1 chain id: %v", err)
	}
	txManagerConfig := &txmgr.Config{
		Backend:                   l1,
		ChainID:                   l1ChainID,
		NumConfirmations:          defaults.NumConfirmations,
		NetworkTimeout:            defaults.NetworkTimeout,
		FeeLimitMultiplier:        defaults.FeeLimitMultiplier,
		ResubmissionTimeout:       defaults.ResubmissionTimeout,
		ReceiptQueryInterval:      defaults.ReceiptQueryInterval,
		TxNotInMempoolTimeout:     defaults.TxNotInMempoolTimeout,
		SafeAbortNonceTooLowCount: defaults.SafeAbortNonceTooLowCount,
		Signer: func(ctx context.Context, address common.Address, tx *ethtypes.Transaction) (*ethtypes.Transaction, error) {
			return opcrypto.PrivateKeySignerFn(op.privKey, l1ChainID)(address, tx)
		},
	}

	if err := op.runProposer(ctx, env, l1, txManagerConfig); err != nil {
		return err
	}

	if err := op.runBatcher(ctx, env, l1, txManagerConfig); err != nil {
		return err
	}
	return nil
}

func (op *OPStack) runNode(ctx context.Context, env *environment.Env) error {
	opNode, err := opnode.New(ctx, &opnode.Config{
		L1: &opnode.L1EndpointConfig{
			L1NodeAddr:     op.l1URL.String(),
			BatchSize:      10,
			MaxConcurrency: 10,
			L1RPCKind:      sources.RPCKindBasic,
		},
		L2: &opnode.L2EndpointConfig{
			L2EngineAddr:      op.engineURL.String(),
			L2EngineJWTSecret: [32]byte{},
		},
		Driver: driver.Config{
			SequencerEnabled: true,
		},
		Rollup: *op.rollupConfig,
		RPC: opnode.RPCConfig{
			ListenAddr: op.nodeURL.Hostname(),
			ListenPort: int(op.nodeURL.PortU16()),
		},
		ConfigPersistence: opnode.DisabledConfigPersistence{},
		Sync: sync.Config{
			SyncMode: sync.CLSync,
		},
	}, op.newLogger("node"), op.newLogger("node-snapshotter"), "v0.1", opnodemetrics.NewMetrics(""))
	if err != nil {
		return fmt.Errorf("new node: %v", err)
	}
	if err := opNode.Start(ctx); err != nil {
		return fmt.Errorf("start node: %v", err)
	}
	env.DeferErr("stop node", func() error {
		return opNode.Stop(context.Background())
	})
	return nil
}

func (op *OPStack) runProposer(ctx context.Context, env *environment.Env, l1Client proposer.L1Client, txManagerConfig *txmgr.Config) error {
	metrics := opproposermetrics.NoopMetrics

	txManager, err := txmgr.NewSimpleTxManagerFromConfig("proposer", op.newLogger("proposer-tx-manager"), metrics, *txManagerConfig)
	if err != nil {
		return fmt.Errorf("new simple tx manager: %v", err)
	}
	env.Defer(txManager.Close)

	rollupProvider, err := dial.NewStaticL2RollupProvider(ctx, op.newLogger("proposer-dialer"), op.nodeURL.String())
	if err != nil {
		return fmt.Errorf("new static l2 rollup provider: %v", err)
	}
	env.Defer(rollupProvider.Close)

	outputSubmitter, err := proposer.NewL2OutputSubmitter(proposer.DriverSetup{
		Log:  op.newLogger("proposer"),
		Metr: metrics,
		Cfg: proposer.ProposerConfig{
			PollInterval:       50 * time.Millisecond,
			NetworkTimeout:     2 * time.Second,
			L2OutputOracleAddr: utils.Ptr(op.l2OutputOracleProxy),
		},
		Txmgr:          txManager,
		L1Client:       l1Client,
		RollupProvider: rollupProvider,
	})
	if err != nil {
		return fmt.Errorf("new l2 output submitter: %v", err)
	}
	if err := outputSubmitter.StartL2OutputSubmitting(); err != nil {
		return fmt.Errorf("start l2 output submitting: %v", err)
	}
	env.DeferErr("stop l2 output submitting", outputSubmitter.StopL2OutputSubmitting)
	return nil
}

func (op *OPStack) runBatcher(ctx context.Context, env *environment.Env, l1Client batcher.L1Client, txManagerConfig *txmgr.Config) error {
	metrics := opbatchermetrics.NoopMetrics

	txManager, err := txmgr.NewSimpleTxManagerFromConfig("batcher", op.newLogger("batcher-tx-manager"), metrics, *txManagerConfig)
	if err != nil {
		return fmt.Errorf("new simple tx manager: %v", err)
	}
	env.Defer(txManager.Close)

	endpointProvider, err := dial.NewStaticL2EndpointProvider(
		ctx,
		op.newLogger("batcher-dialer"),
		op.engineURL.String(),
		op.nodeURL.String(),
	)
	if err != nil {
		return fmt.Errorf("new static l2 endpoint provider: %v", err)
	}
	env.Defer(endpointProvider.Close)

	batchSubmitter := batcher.NewBatchSubmitter(batcher.DriverSetup{
		Log:          op.newLogger("batcher"),
		Metr:         metrics,
		RollupConfig: op.rollupConfig,
		Config: batcher.BatcherConfig{
			NetworkTimeout: 10 * time.Second,
			PollInterval:   50 * time.Millisecond,
		},
		Txmgr:            txManager,
		L1Client:         l1Client,
		EndpointProvider: endpointProvider,
		ChannelConfig: batcher.ChannelConfig{
			SeqWindowSize:  op.rollupConfig.SeqWindowSize,
			ChannelTimeout: op.rollupConfig.ChannelTimeout,
			// These values are taken from the op-e2e test configs.
			MaxChannelDuration: 1,
			SubSafetyMargin:    4,
			MaxFrameSize:       math.MaxUint64,
			CompressorConfig: compressor.Config{
				TargetOutputSize: 100_000,
				ApproxComprRatio: 0.4,
			},
		},
	})
	if err := batchSubmitter.StartBatchSubmitting(); err != nil {
		return fmt.Errorf("start batch submitting: %v", err)
	}
	/*
		There appears to be a deadlock in StopBatchSubmitting.
		This was most likely fixed in a more recent OP-stack version, based on the significant diff.
		env.DeferErr( "stop batch submitting", func() error {
			return batchSubmitter.StopBatchSubmitting(ctx)
		})
	*/
	return nil
}

func (op *OPStack) newLogger(name string) log.Logger {
	return log.NewLogger(&logHandler{
		eventListener: op.eventListener,
	}).With("monomer-e2e-component", name)
}

type logHandler struct {
	attrs         []slog.Attr
	eventListener OPEventListener
}

var _ slog.Handler = (*logHandler)(nil)

func (h *logHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *logHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam
	if h.attrs == nil {
		h.eventListener.Log(r)
	} else {
		newRecord := r.Clone()
		newRecord.AddAttrs(h.attrs...)
		h.eventListener.Log(newRecord)
	}
	return nil
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{
		attrs:         append(h.attrs, attrs...),
		eventListener: h.eventListener,
	}
}

func (h *logHandler) WithGroup(name string) slog.Handler {
	return h
}

package peptide

import (
	"strings"

	errs "cosmossdk.io/errors"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkErr "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/gogoproto/proto"
)

// RunMsgs execute a list of L1 Msgs that are not signed by any L2 account
// NOTE: these msgs must be executed at the beginning of a block, before any L2 txs are executed
//
// Logic is similar to ABCI app tx execution, but adjusted for L1 deposit txs
// TODO: cache context in case of revert
func (a *PeptideApp) RunMsgs(ctx sdk.Context, msgs ...sdk.Msg) (*sdk.Result, error) {
	msgLogs := make(sdk.ABCIMessageLogs, 0, len(msgs))
	events := sdk.EmptyEvents()
	var msgResponses []*codectypes.Any

	_, _, _ = msgLogs, events, msgResponses

	for i, msg := range msgs {
		handler := a.MsgServiceRouter().Handler(msg)
		if handler == nil {
			return nil, errs.Wrapf(sdkErr.ErrInvalidRequest, "unrecognized %s message type: %T", sdk.MsgTypeURL(msg), msg)
		}
		msgResult, err := handler(ctx, msg)
		if err != nil {
			return nil, errs.Wrapf(err, "failed to execute message; message index: %d", i)
		}

		msgEvents := createEvents(msgResult.GetEvents(), msg)
		events = events.AppendEvents(msgEvents)

		// aggregate msg responses into a slice
		if len(msgResult.MsgResponses) > 0 {
			msgResponse := msgResult.MsgResponses[0]
			if msgResponse == nil {
				return nil, sdkErr.ErrLogic.Wrapf("got nil Msg response at index %d for msg %s", i, sdk.MsgTypeURL(msg))
			}
			msgResponses = append(msgResponses, msgResponse)
		}

		msgLogs = append(msgLogs, sdk.NewABCIMessageLog(uint32(i), msgResult.Log, msgEvents))
	}

	data, err := proto.Marshal(&sdk.TxMsgData{MsgResponses: msgResponses})
	if err != nil {
		return nil, errs.Wrapf(err, "failed to marshal tx msg data")
	}

	return &sdk.Result{
		Data:         data,
		Log:          strings.TrimSpace(msgLogs.String()),
		Events:       events.ToABCIEvents(),
		MsgResponses: msgResponses,
	}, nil
}

func createEvents(events sdk.Events, msg sdk.Msg) sdk.Events {
	eventMsgName := sdk.MsgTypeURL(msg)
	msgEvent := sdk.NewEvent(sdk.EventTypeMessage, sdk.NewAttribute(sdk.AttributeKeyAction, eventMsgName))

	// we set the signer attribute as the sender
	if len(msg.GetSigners()) > 0 && !msg.GetSigners()[0].Empty() {
		msgEvent = msgEvent.AppendAttributes(sdk.NewAttribute(sdk.AttributeKeySender, msg.GetSigners()[0].String()))
	}

	// verify that events have no module attribute set
	if _, found := events.GetAttributes(sdk.AttributeKeyModule); !found {
		// here we assume that routes module name is the second element of the route
		// e.g. "cosmos.bank.v1beta1.MsgSend" => "bank"
		moduleName := strings.Split(eventMsgName, ".")
		if len(moduleName) > 1 {
			msgEvent = msgEvent.AppendAttributes(sdk.NewAttribute(sdk.AttributeKeyModule, moduleName[1]))
		}
	}

	return sdk.Events{msgEvent}.AppendEvents(events)
}

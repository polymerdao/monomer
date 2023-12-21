package types

import (
	"math/big"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common"

	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// only required for legacy Amino codec
func RegisterCodec(cdc *codec.LegacyAmino) {
}

// RegisterInterfaces registers the module's interface types
// Msg types are auto registered by msgservice.
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}

// ToL1BlockInfoResponse converts a derive.L1BlockInfo to QueryL1BlockInfoResponse.
func ToL1BlockInfoResponse(l1blockInfo derive.L1BlockInfo) *QueryL1BlockInfoResponse {
	return &QueryL1BlockInfoResponse{
		Number:         l1blockInfo.Number,
		Time:           l1blockInfo.Time,
		BaseFee:        l1blockInfo.BaseFee.Bytes(),
		BlockHash:      l1blockInfo.BlockHash.Bytes(),
		SequenceNumber: l1blockInfo.SequenceNumber,
		BatcherAddr:    l1blockInfo.BatcherAddr.Bytes(),
		L1FeeOverhead:  l1blockInfo.L1FeeOverhead[:],
		L1FeeScalar:    l1blockInfo.L1FeeScalar[:],
	}
}

// FromL1BlockInfoResponse converts a QueryL1BlockInfoResponse to derive.L1BlockInfo.
func FromL1BlockInfoResponse(resp *QueryL1BlockInfoResponse) derive.L1BlockInfo {
	info := derive.L1BlockInfo{
		Number:         resp.Number,
		Time:           resp.Time,
		BaseFee:        big.NewInt(0).SetBytes(resp.BaseFee),
		SequenceNumber: resp.SequenceNumber,
		BatcherAddr:    common.BytesToAddress(resp.BatcherAddr),
	}
	copy(info.BlockHash[:], resp.BlockHash)
	copy(info.L1FeeOverhead[:], resp.L1FeeOverhead)
	copy(info.L1FeeScalar[:], resp.L1FeeScalar)
	return info
}

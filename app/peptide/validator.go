package peptide

import (
	"fmt"
	"time"

	"cosmossdk.io/math"
	tmtypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/samber/lo"
)

// NewDefaultValidatorSet returns a ValidatorSet with one random Validator
func NewDefaultValidatorSet() *tmtypes.ValidatorSet {
	return NewValidatorSet(1)
}

// NewValidatorSet returns a default ValidatorSet with `count` random Validators
func NewValidatorSet(count int) *tmtypes.ValidatorSet {
	vals := make([]*tmtypes.Validator, count)
	for i := 0; i < count; i++ {
		vals[i] = newMockValidator()
	}
	return tmtypes.NewValidatorSet(vals)
}

func newMockValidator() *tmtypes.Validator {
	privVal := tmtypes.NewMockPV()
	pubKey := lo.Must(privVal.GetPubKey())
	return tmtypes.NewValidator(pubKey, 1)
}

func randomValidator() (string, *codectypes.Any) {
	privVal := tmtypes.NewMockPV()
	pubKey := lo.Must(cryptocodec.FromTmPubKeyInterface(privVal.PrivKey.PubKey()))
	return sdk.ValAddress(pubKey.Address()).String(), lo.Must(codectypes.NewAnyWithValue(pubKey))
}

type SingleDelegation struct {
	AccountIndex int
	Amount       math.Int
}

type ValidatorDelegation []SingleDelegation

type ValidatorSetDelegation []ValidatorDelegation

// sdk.DefaultPowerReduction = 1000000 (10^6)

func newStakingValidators(
	validators, delegators SignerAccounts,
	delegations ValidatorSetDelegation,
) (stakingtypes.Validators, stakingtypes.Delegations, sdk.Int) {
	if len(delegations) != len(validators) {
		panic(
			fmt.Errorf("number of delegations must equal validators: but %d != %d", len(delegations), len(validators)),
		)
	}

	var realDels stakingtypes.Delegations
	totalBond := sdk.NewInt(0)

	createStakingValidator := func(valDel ValidatorDelegation, validator *SignerAccount) stakingtypes.Validator {
		valBond := sdk.NewInt(0)
		status := stakingtypes.Unbonded
		valOpAddr := sdk.ValAddress(validator.PubKey().Address()).String()
		for _, del := range valDel {
			valBond = valBond.Add(del.Amount)
			realDels = append(
				realDels,
				// stakingtypes.NewDelegation(
				// 	delegators[del.AccountIndex].GetAddress(),
				// 	validator.PubKey().Bytes(),
				// 	del.Amount.ToDec(),
				// ),
				stakingtypes.Delegation{
					DelegatorAddress: delegators[del.AccountIndex].GetAddress().String(),
					ValidatorAddress: valOpAddr,
					Shares:           sdk.NewDecFromInt(del.Amount),
				},
			)
			status = stakingtypes.Bonded
		}

		totalBond = totalBond.Add(valBond)
		return stakingtypes.Validator{
			OperatorAddress:   sdk.ValAddress(validator.PubKey().Address()).String(),
			ConsensusPubkey:   lo.Must(codectypes.NewAnyWithValue(validator.PubKey())),
			Jailed:            false,
			Status:            status,
			Tokens:            valBond,
			DelegatorShares:   sdk.NewDecFromInt(valBond),
			Description:       stakingtypes.Description{},
			UnbondingHeight:   int64(0),
			UnbondingTime:     time.Unix(0, 0).UTC(),
			Commission:        stakingtypes.NewCommission(sdk.ZeroDec(), sdk.ZeroDec(), sdk.ZeroDec()),
			MinSelfDelegation: sdk.ZeroInt(),
		}
	}

	stakingValidators := make(stakingtypes.Validators, len(validators))
	for i, delegation := range delegations {
		stakingValidators[i] = createStakingValidator(delegation, validators[i])
	}
	return stakingValidators, realDels, totalBond
}

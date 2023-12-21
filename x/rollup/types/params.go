package types

import (
	fmt "fmt"
	"strings"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"gopkg.in/yaml.v2"
)

var _ paramtypes.ParamSet = (*Params)(nil)

var (
	KeyAllowedErc20Tokens = []byte("AllowedERC20Tokens")
)

// ParamKeyTable the param key table for launch module
func ParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&Params{})
}

// NewParams creates a new Params instance
func NewParams(
	allowedErc20Tokens ...string,
) Params {
	return Params{
		AllowedErc20Tokens: allowedErc20Tokens,
	}
}

// DefaultParams returns a default set of parameters
func DefaultParams() Params {
	return NewParams()
}

// Validate validates the set of params
func (p Params) Validate() error {
	return validateTokens(p.AllowedErc20Tokens)
}

// String implements the Stringer interface.
func (p Params) String() string {
	out, _ := yaml.Marshal(p)
	return string(out)
}

// ParamSetPairs implements params.ParamSet
func (p *Params) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{
		paramtypes.NewParamSetPair(KeyAllowedErc20Tokens, p.AllowedErc20Tokens, validateTokens),
	}
}

func validateTokens(i interface{}) error {
	tokens, ok := i.([]string)
	if !ok {
		return fmt.Errorf("invalid parameter type: %T", i)
	}

	for i, token := range tokens {
		if strings.TrimSpace(token) == "" {
			return fmt.Errorf("token %d cannot be blank", i)
		}
	}

	return nil
}

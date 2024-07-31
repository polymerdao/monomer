// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package bindings

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
)

// L2ApplicationStateRootProviderMetaData contains all meta data concerning the L2ApplicationStateRootProvider contract.
var L2ApplicationStateRootProviderMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"l2ApplicationStateRoot\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"setL2ApplicationStateRoot\",\"inputs\":[{\"name\":\"_l2ApplicationStateRoot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"}]",
	Bin: "0x6080604052348015600f57600080fd5b506004361060325760003560e01c80633135b57e14603757806389b72f86146051575b600080fd5b603f60005481565b60405190815260200160405180910390f35b6061605c3660046063565b600055565b005b600060208284031215607457600080fd5b503591905056fea26469706673582212205323d48cb3c9db241bd671b86ca8e28dd31c3921a9fa92dc80c40a311e5067a564736f6c63430008190033",
}

// L2ApplicationStateRootProviderABI is the input ABI used to generate the binding from.
// Deprecated: Use L2ApplicationStateRootProviderMetaData.ABI instead.
var L2ApplicationStateRootProviderABI = L2ApplicationStateRootProviderMetaData.ABI

// L2ApplicationStateRootProviderBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use L2ApplicationStateRootProviderMetaData.Bin instead.
var L2ApplicationStateRootProviderBin = L2ApplicationStateRootProviderMetaData.Bin

// DeployL2ApplicationStateRootProvider deploys a new Ethereum contract, binding an instance of L2ApplicationStateRootProvider to it.
func DeployL2ApplicationStateRootProvider(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *L2ApplicationStateRootProvider, error) {
	parsed, err := L2ApplicationStateRootProviderMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(L2ApplicationStateRootProviderBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &L2ApplicationStateRootProvider{L2ApplicationStateRootProviderCaller: L2ApplicationStateRootProviderCaller{contract: contract}, L2ApplicationStateRootProviderTransactor: L2ApplicationStateRootProviderTransactor{contract: contract}, L2ApplicationStateRootProviderFilterer: L2ApplicationStateRootProviderFilterer{contract: contract}}, nil
}

// L2ApplicationStateRootProvider is an auto generated Go binding around an Ethereum contract.
type L2ApplicationStateRootProvider struct {
	L2ApplicationStateRootProviderCaller     // Read-only binding to the contract
	L2ApplicationStateRootProviderTransactor // Write-only binding to the contract
	L2ApplicationStateRootProviderFilterer   // Log filterer for contract events
}

// L2ApplicationStateRootProviderCaller is an auto generated read-only Go binding around an Ethereum contract.
type L2ApplicationStateRootProviderCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// L2ApplicationStateRootProviderTransactor is an auto generated write-only Go binding around an Ethereum contract.
type L2ApplicationStateRootProviderTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// L2ApplicationStateRootProviderFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type L2ApplicationStateRootProviderFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// L2ApplicationStateRootProviderSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type L2ApplicationStateRootProviderSession struct {
	Contract     *L2ApplicationStateRootProvider // Generic contract binding to set the session for
	CallOpts     bind.CallOpts                   // Call options to use throughout this session
	TransactOpts bind.TransactOpts               // Transaction auth options to use throughout this session
}

// L2ApplicationStateRootProviderCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type L2ApplicationStateRootProviderCallerSession struct {
	Contract *L2ApplicationStateRootProviderCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                         // Call options to use throughout this session
}

// L2ApplicationStateRootProviderTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type L2ApplicationStateRootProviderTransactorSession struct {
	Contract     *L2ApplicationStateRootProviderTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                         // Transaction auth options to use throughout this session
}

// L2ApplicationStateRootProviderRaw is an auto generated low-level Go binding around an Ethereum contract.
type L2ApplicationStateRootProviderRaw struct {
	Contract *L2ApplicationStateRootProvider // Generic contract binding to access the raw methods on
}

// L2ApplicationStateRootProviderCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type L2ApplicationStateRootProviderCallerRaw struct {
	Contract *L2ApplicationStateRootProviderCaller // Generic read-only contract binding to access the raw methods on
}

// L2ApplicationStateRootProviderTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type L2ApplicationStateRootProviderTransactorRaw struct {
	Contract *L2ApplicationStateRootProviderTransactor // Generic write-only contract binding to access the raw methods on
}

// NewL2ApplicationStateRootProvider creates a new instance of L2ApplicationStateRootProvider, bound to a specific deployed contract.
func NewL2ApplicationStateRootProvider(address common.Address, backend bind.ContractBackend) (*L2ApplicationStateRootProvider, error) {
	contract, err := bindL2ApplicationStateRootProvider(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &L2ApplicationStateRootProvider{L2ApplicationStateRootProviderCaller: L2ApplicationStateRootProviderCaller{contract: contract}, L2ApplicationStateRootProviderTransactor: L2ApplicationStateRootProviderTransactor{contract: contract}, L2ApplicationStateRootProviderFilterer: L2ApplicationStateRootProviderFilterer{contract: contract}}, nil
}

// NewL2ApplicationStateRootProviderCaller creates a new read-only instance of L2ApplicationStateRootProvider, bound to a specific deployed contract.
func NewL2ApplicationStateRootProviderCaller(address common.Address, caller bind.ContractCaller) (*L2ApplicationStateRootProviderCaller, error) {
	contract, err := bindL2ApplicationStateRootProvider(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &L2ApplicationStateRootProviderCaller{contract: contract}, nil
}

// NewL2ApplicationStateRootProviderTransactor creates a new write-only instance of L2ApplicationStateRootProvider, bound to a specific deployed contract.
func NewL2ApplicationStateRootProviderTransactor(address common.Address, transactor bind.ContractTransactor) (*L2ApplicationStateRootProviderTransactor, error) {
	contract, err := bindL2ApplicationStateRootProvider(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &L2ApplicationStateRootProviderTransactor{contract: contract}, nil
}

// NewL2ApplicationStateRootProviderFilterer creates a new log filterer instance of L2ApplicationStateRootProvider, bound to a specific deployed contract.
func NewL2ApplicationStateRootProviderFilterer(address common.Address, filterer bind.ContractFilterer) (*L2ApplicationStateRootProviderFilterer, error) {
	contract, err := bindL2ApplicationStateRootProvider(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &L2ApplicationStateRootProviderFilterer{contract: contract}, nil
}

// bindL2ApplicationStateRootProvider binds a generic wrapper to an already deployed contract.
func bindL2ApplicationStateRootProvider(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(L2ApplicationStateRootProviderABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _L2ApplicationStateRootProvider.Contract.L2ApplicationStateRootProviderCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _L2ApplicationStateRootProvider.Contract.L2ApplicationStateRootProviderTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _L2ApplicationStateRootProvider.Contract.L2ApplicationStateRootProviderTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _L2ApplicationStateRootProvider.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _L2ApplicationStateRootProvider.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _L2ApplicationStateRootProvider.Contract.contract.Transact(opts, method, params...)
}

// L2ApplicationStateRoot is a free data retrieval call binding the contract method 0x3135b57e.
//
// Solidity: function l2ApplicationStateRoot() view returns(bytes32)
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderCaller) L2ApplicationStateRoot(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _L2ApplicationStateRootProvider.contract.Call(opts, &out, "l2ApplicationStateRoot")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// L2ApplicationStateRoot is a free data retrieval call binding the contract method 0x3135b57e.
//
// Solidity: function l2ApplicationStateRoot() view returns(bytes32)
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderSession) L2ApplicationStateRoot() ([32]byte, error) {
	return _L2ApplicationStateRootProvider.Contract.L2ApplicationStateRoot(&_L2ApplicationStateRootProvider.CallOpts)
}

// L2ApplicationStateRoot is a free data retrieval call binding the contract method 0x3135b57e.
//
// Solidity: function l2ApplicationStateRoot() view returns(bytes32)
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderCallerSession) L2ApplicationStateRoot() ([32]byte, error) {
	return _L2ApplicationStateRootProvider.Contract.L2ApplicationStateRoot(&_L2ApplicationStateRootProvider.CallOpts)
}

// SetL2ApplicationStateRoot is a paid mutator transaction binding the contract method 0x89b72f86.
//
// Solidity: function setL2ApplicationStateRoot(bytes32 _l2ApplicationStateRoot) returns()
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderTransactor) SetL2ApplicationStateRoot(opts *bind.TransactOpts, _l2ApplicationStateRoot [32]byte) (*types.Transaction, error) {
	return _L2ApplicationStateRootProvider.contract.Transact(opts, "setL2ApplicationStateRoot", _l2ApplicationStateRoot)
}

// SetL2ApplicationStateRoot is a paid mutator transaction binding the contract method 0x89b72f86.
//
// Solidity: function setL2ApplicationStateRoot(bytes32 _l2ApplicationStateRoot) returns()
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderSession) SetL2ApplicationStateRoot(_l2ApplicationStateRoot [32]byte) (*types.Transaction, error) {
	return _L2ApplicationStateRootProvider.Contract.SetL2ApplicationStateRoot(&_L2ApplicationStateRootProvider.TransactOpts, _l2ApplicationStateRoot)
}

// SetL2ApplicationStateRoot is a paid mutator transaction binding the contract method 0x89b72f86.
//
// Solidity: function setL2ApplicationStateRoot(bytes32 _l2ApplicationStateRoot) returns()
func (_L2ApplicationStateRootProvider *L2ApplicationStateRootProviderTransactorSession) SetL2ApplicationStateRoot(_l2ApplicationStateRoot [32]byte) (*types.Transaction, error) {
	return _L2ApplicationStateRootProvider.Contract.SetL2ApplicationStateRoot(&_L2ApplicationStateRootProvider.TransactOpts, _l2ApplicationStateRoot)
}

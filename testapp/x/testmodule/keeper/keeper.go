package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/core/store"
	testappv1 "github.com/polymerdao/monomer/gen/testapp/v1"
)

type Keeper struct {
	storeService store.KVStoreService
}

func New(storeService store.KVStoreService) *Keeper {
	return &Keeper{
		storeService: storeService,
	}
}

func (m *Keeper) InitGenesis(ctx context.Context, kvs map[string]string) error {
	store := m.storeService.OpenKVStore(ctx)
	for k, v := range kvs {
		if err := store.Set([]byte(k), []byte(v)); err != nil {
			return fmt.Errorf("set: %v", err)
		}
	}
	return nil
}

func (m *Keeper) Get(ctx context.Context, req *testappv1.GetRequest) (*testappv1.GetResponse, error) {
	key := req.GetKey()
	if key == "" {
		return nil, errors.New("empty key")
	}
	valueBytes, err := m.storeService.OpenKVStore(ctx).Get([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("get: %v", err)
	}
	var value string
	if valueBytes == nil {
		value = ""
	} else {
		value = string(valueBytes)
	}
	return &testappv1.GetResponse{
		Value: value,
	}, nil
}

func (m *Keeper) Set(ctx context.Context, req *testappv1.SetRequest) (*testappv1.SetResponse, error) {
	key := req.GetKey()
	if err := m.storeService.OpenKVStore(ctx).Set([]byte(key), []byte(req.GetValue())); err != nil {
		return nil, fmt.Errorf("set: %v", err)
	}
	return &testappv1.SetResponse{}, nil
}

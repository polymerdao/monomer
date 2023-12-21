package peptide

import (
	"fmt"
	"log"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/samber/lo"
)

func panicIfError(err error) {
	if err != nil {
		panic(err)
	}
}

func panicIf(condition bool, message string, args ...interface{}) {
	if condition {
		panic(fmt.Errorf(message, args...))
	}
}

func MustIntFromString(s string) sdk.Int {
	i, ok := sdk.NewIntFromString(s)
	panicIf(!ok, "cannot convert '%s' to sdk.Int", s)
	return i
}

type Request interface {
	Marshal() ([]byte, error)
}

type Response interface {
	Unmarshal(resp []byte) error
}

type QueryableApp interface {
	Query(req abci.RequestQuery) (res abci.ResponseQuery)
}

type QueryClient interface {
	QuerySync(req abci.RequestQuery) (*abci.ResponseQuery, error)
}

// MustGetResponse is a helper function that sends a query to a chain App and unmarshals the response into the provided response type.
// It panics if the query fails or if the response cannot be unmarshaled.
func MustGetResponse[T Response](resp T, app QueryableApp, req Request, url string) (T, uint32) {
	queryResp := app.Query(abci.RequestQuery{Data: lo.Must(req.Marshal()), Path: url})
	// panics if abci query response cannot be marshalled to type TResp
	lo.Must0(resp.Unmarshal(queryResp.Value))
	if queryResp.Code != 0 {
		log.Panicf("failed query with url: %s, response: %s", url, lo.Must(queryResp.MarshalJSON()))
	}
	return resp, queryResp.Code
}

// MustGetResponseWithHeight is a helper function that sends a query to a chain App and unmarshals the response into the
// provided response type.
// It panics if the query fails or if the response cannot be unmarshaled.
func MustGetResponseWithHeight[T Response](resp T, app QueryableApp, req Request, url string, height int64) T {
	queryResp := app.Query(abci.RequestQuery{Data: lo.Must(req.Marshal()), Path: url, Height: height})
	// panics if abci query response cannot be marshalled to type TResp
	lo.Must0(resp.Unmarshal(queryResp.Value))
	if queryResp.Code != 0 {
		log.Panicf("failed query with url: %s, response: %s", url, lo.Must(queryResp.MarshalJSON()))
	}
	return resp
}

// MustGetClientResponse is a helper function that sends a query to an abci query client  and unmarshals the response into the provided response type.
// It panics if the query fails or if the response cannot be unmarshaled.
func MustGetClientResponse[T Response](resp T, app QueryClient, req Request, url string) T {
	queryResp, err := app.QuerySync(abci.RequestQuery{Data: lo.Must(req.Marshal()), Path: url})
	if err != nil {
		log.Panicf("failed query with url: %s, error: %s", url, err)
	}
	// panics if abci query response cannot be marshalled to type TResp
	lo.Must0(resp.Unmarshal(queryResp.Value))
	if queryResp.Code != 0 {
		log.Panicf("failed query with url: %s, response: %s", url, lo.Must(queryResp.MarshalJSON()))
	}
	return resp
}

// QueryApp is a helper function that sends a query to an abci query client and unmarshals the response into the provided response type.
// It returns the response and an error if the query fails or if the response cannot be unmarshaled.
func QueryApp[T Response](resp T, app QueryClient, req Request, url string, height int64) (T, error) {
	// if req fails to marshal, returns an error
	reqBytes, err := req.Marshal()
	if err != nil {
		return resp, fmt.Errorf("failed to marshal request for query %s due to %w", url, err)
	}
	queryResp, err := app.QuerySync(abci.RequestQuery{Data: reqBytes, Path: url, Height: height})
	if err != nil {
		return resp, fmt.Errorf("failed query with url: %s, error: %s", url, err)
	}
	// returns an error if abci query response cannot be marshalled to response type
	if err := resp.Unmarshal(queryResp.Value); err != nil {
		return resp, err
	}
	if queryResp.Code != 0 {
		return resp, fmt.Errorf("failed query with url: %s, response: %s", url, lo.Must(queryResp.MarshalJSON()))
	}
	return resp, nil
}

// MustUnmarshalResultData unmarshals sdk.Result data to a provided response type or panics.
func MustUnmarshalResultData(codec codec.Codec, data []byte, ptr codec.ProtoMarshaler) {
	codec.MustUnmarshal(data, ptr)
}

// UnmarshalResultData unmarshals sdk.Result data to a provided response type.
func UnmarshalResultData(codec codec.Codec, data []byte, ptr codec.ProtoMarshaler) error {
	return codec.Unmarshal(data, ptr)
}

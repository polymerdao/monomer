package node

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/polymerdao/monomer/utils"
	"github.com/sourcegraph/conc"
)

type httpService struct {
	srv      *http.Server
	listener net.Listener
}

func makeHTTPService(handler http.Handler, listener net.Listener) *httpService {
	return &httpService{
		srv: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 30 * time.Second,
		},
		listener: listener,
	}
}

func (h *httpService) Run(parentCtx context.Context) error {
	ctx, cancel := context.WithCancelCause(parentCtx)
	defer cancel(nil)
	var wg conc.WaitGroup
	defer wg.Wait()
	wg.Go(func() {
		if err := h.srv.Serve(h.listener); !errors.Is(err, http.ErrServerClosed) {
			cancel(fmt.Errorf("serve http: %v", err))
		}
	})
	<-ctx.Done()
	if cause := utils.Cause(ctx); cause != nil {
		return cause
	}
	if err := h.srv.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("shutdown http server: %v", err)
	}
	return nil
}

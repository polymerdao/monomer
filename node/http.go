package node

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

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

func (h *httpService) Run(ctx context.Context) error {
	errCh := make(chan error)
	defer close(errCh)
	var wg conc.WaitGroup
	defer wg.Wait()
	wg.Go(func() {
		if err := h.srv.Serve(h.listener); !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("serve: %v", err)
		}
	})
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		if err := h.srv.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("shutdown: %v", err)
		}
	}
	return nil
}

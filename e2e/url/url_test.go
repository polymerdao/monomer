package url_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/stretchr/testify/require"
)

func parseURL(t *testing.T, urlString string) *url.URL {
	got, err := url.Parse(urlString)
	require.NoError(t, err)
	return got
}

func TestParse(t *testing.T) {
	tests := map[string]struct {
		errMsg string
		url    *url.URL
	}{
		"nil url": {
			errMsg: "url is nil",
		},
		"valid url with port included": {
			url: parseURL(t, "http://127.0.0.1:10"),
		},
		"relative url": {
			errMsg: "url is not absolute (does not contain scheme)",
			url:    parseURL(t, "/"),
		},
		"valid url with port excluded": {
			url: parseURL(t, "http://127.0.0.1"),
		},
	}

	for description, test := range tests {
		t.Run(description, func(t *testing.T) {
			gotURL, err := e2eurl.Parse(test.url)
			if test.errMsg != "" {
				require.ErrorContains(t, err, test.errMsg)
				return
			}
			// Port is present in gotURL.Host.
			_, _, err = net.SplitHostPort(gotURL.Host())
			require.NoError(t, err)
		})
	}
}

func TestIsReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	gotURL, err := e2eurl.Parse(parseURL(t, srv.URL))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	require.True(t, gotURL.IsReachable(ctx))

	cancel()
	require.False(t, gotURL.IsReachable(ctx))
}

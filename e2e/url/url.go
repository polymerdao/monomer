package url

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/polymerdao/monomer/utils"
	"tailscale.com/logtail/backoff"
)

type URL struct {
	url  *url.URL
	port uint16
}

func Parse(stdURL *url.URL) (*URL, error) {
	if stdURL == nil {
		return nil, errors.New("url is nil")
	}

	if !stdURL.IsAbs() {
		return nil, fmt.Errorf("url is not absolute (does not contain scheme) %s", stdURL)
	}

	// Make a deep copy of the url.
	newURL := utils.Ptr(*stdURL)
	if stdURL.User != nil {
		newURL.User = utils.Ptr(*stdURL.User)
	}

	var portU16 uint16
	portString := stdURL.Port()
	if portString == "" {
		portInt, err := net.LookupPort("tcp", stdURL.Scheme)
		if err != nil {
			return nil, fmt.Errorf("lookup port: %v", err)
		}
		portU16 = uint16(portInt)
		portString = strconv.FormatInt(int64(portInt), 10)
	} else {
		portU64, err := strconv.ParseUint(portString, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("parse port string: %v", err)
		}
		portU16 = uint16(portU64)
	}

	// Ensure the port is present.
	newURL.Host = net.JoinHostPort(stdURL.Hostname(), portString)

	return &URL{
		url:  newURL,
		port: portU16,
	}, nil
}

func (u *URL) Host() string {
	return u.url.Host
}

func (u *URL) Hostname() string {
	return u.url.Hostname()
}

func (u *URL) PortU16() uint16 {
	return u.port
}

func (u *URL) Port() string {
	return u.url.Port()
}

func (u *URL) String() string {
	return u.url.String()
}

func (u *URL) IsReachable(ctx context.Context) bool {
	// TODO Use op-service/backoff when we upgrade to latest OP stack version.
	// Ideally, we would accept a backoff timer. That would give the caller the opportunity
	// to use the same backoff timer for a single address. This is probably overkill,
	// especially since this is only used in for e2e testing purposes.
	backoffTimer := backoff.NewBackoff("", func(_ string, _ ...any) {}, time.Second)
	var d net.Dialer
	for ctx.Err() == nil {
		conn, err := d.DialContext(ctx, "tcp", u.Host())
		if err == nil {
			conn.Close()
			return true
		}
		backoffTimer.BackOff(ctx, err)
	}
	return false
}

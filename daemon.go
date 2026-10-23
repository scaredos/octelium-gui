package main

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"github.com/octelium/octelium/apis/client/daemonv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// daemon is a client of the privileged `octelium daemon` service, which owns
// the credentials and the WireGuard device so this app never needs admin rights.
type daemon struct{ daemonv1.MainServiceClient }

func dialDaemon() (*daemon, error) {
	addr := os.Getenv("OCTELIUM_DAEMON_SOCKET")
	if addr == "" {
		addr = defaultDaemonAddr
	}
	// The daemon identifies the caller from the pipe/socket peer, so there is
	// nothing for the client side of the handshake to do.
	conn, err := grpc.NewClient("passthrough:///octelium-daemon",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return dial(ctx, addr)
		}))
	if err != nil {
		return nil, err
	}
	return &daemon{daemonv1.NewMainServiceClient(conn)}, nil
}

var pollInterval = 500 * time.Millisecond

// run starts an Operation and blocks until it finishes, handing any URL the
// daemon wants the user to visit (browser login) to openURL once.
func (d *daemon) run(ctx context.Context, start func(context.Context) (*daemonv1.Operation, error), openURL func(string)) error {
	op, err := start(ctx)
	opened := ""
	for {
		if err != nil {
			return err
		}
		switch op.GetState() {
		case daemonv1.Operation_SUCCEEDED:
			return nil
		case daemonv1.Operation_FAILED, daemonv1.Operation_CANCELED:
			if msg := op.GetError().GetMessage(); msg != "" {
				return errors.New(msg)
			}
			return errors.New(strings.ToLower(op.GetState().String()))
		case daemonv1.Operation_WAITING_FOR_USER:
			if u := op.GetAction().GetOpenURL().GetUrl(); u != "" && u != opened {
				opened = u
				openURL(u)
			}
		}
		select {
		case <-ctx.Done():
			d.CancelOperation(context.Background(), &daemonv1.CancelOperationRequest{Id: op.GetId()})
			return ctx.Err()
		case <-time.After(pollInterval):
		}
		op, err = d.GetOperation(ctx, &daemonv1.GetOperationRequest{Id: op.GetId()})
	}
}

// watch streams the state of domain to fn until ctx ends, redialing every 2s
// while the daemon is unreachable. fn gets a nil state for an unknown domain.
func (d *daemon) watch(ctx context.Context, domain func() string, fn func(*daemonv1.DomainState, error)) {
	for ctx.Err() == nil {
		stream, err := d.WatchStatus(ctx, &daemonv1.WatchStatusRequest{})
		for err == nil {
			var st *daemonv1.GetStatusResponse
			if st, err = stream.Recv(); err == nil {
				fn(findDomain(st, domain()), nil)
			}
		}
		if ctx.Err() == nil {
			fn(nil, err)
			time.Sleep(2 * time.Second)
		}
	}
}

func findDomain(st *daemonv1.GetStatusResponse, domain string) *daemonv1.DomainState {
	for _, ds := range st.GetDomains() {
		if ds.GetDomain() == domain {
			return ds
		}
	}
	return nil
}

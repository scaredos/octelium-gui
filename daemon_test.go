package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/octelium/octelium/apis/client/daemonv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// fakeDaemon walks an Operation through the given states, one per poll.
type fakeDaemon struct {
	daemonv1.UnimplementedMainServiceServer
	states []*daemonv1.Operation
	polls  int
}

func (f *fakeDaemon) Authenticate(context.Context, *daemonv1.AuthenticateRequest) (*daemonv1.Operation, error) {
	return f.states[0], nil
}

func (f *fakeDaemon) GetOperation(context.Context, *daemonv1.GetOperationRequest) (*daemonv1.Operation, error) {
	f.polls++
	return f.states[min(f.polls, len(f.states)-1)], nil
}

func newTestDaemon(t *testing.T, f *fakeDaemon) *daemon {
	lis := bufconn.Listen(1 << 16)
	srv := grpc.NewServer()
	daemonv1.RegisterMainServiceServer(srv, f)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	return &daemon{daemonv1.NewMainServiceClient(conn)}
}

func TestRun(t *testing.T) {
	pollInterval = time.Millisecond
	waiting := &daemonv1.Operation{Id: "1", State: daemonv1.Operation_WAITING_FOR_USER,
		Action: &daemonv1.Action{Type: &daemonv1.Action_OpenURL_{OpenURL: &daemonv1.Action_OpenURL{Url: "https://example.com/login"}}}}

	for _, tc := range []struct {
		name    string
		last    *daemonv1.Operation
		wantErr string
	}{
		{"success", &daemonv1.Operation{State: daemonv1.Operation_SUCCEEDED}, ""},
		{"failure", &daemonv1.Operation{State: daemonv1.Operation_FAILED, Error: &daemonv1.Error{Message: "denied"}}, "denied"},
		{"canceled", &daemonv1.Operation{State: daemonv1.Operation_CANCELED}, "canceled"},
	} {
		d := newTestDaemon(t, &fakeDaemon{states: []*daemonv1.Operation{waiting, waiting, waiting, tc.last}})
		var opened []string
		err := d.run(context.Background(), func(ctx context.Context) (*daemonv1.Operation, error) {
			return d.Authenticate(ctx, &daemonv1.AuthenticateRequest{})
		}, func(u string) { opened = append(opened, u) })

		if got := errString(err); got != tc.wantErr {
			t.Errorf("%s: err = %q, want %q", tc.name, got, tc.wantErr)
		}
		if len(opened) != 1 {
			t.Errorf("%s: browser opened %d times, want once", tc.name, len(opened))
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

package main

import (
	"context"
	"net"
	"os/user"

	"github.com/Microsoft/go-winio"
)

const defaultDaemonAddr = `\\.\pipe\octelium-daemon`

func dial(ctx context.Context, addr string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, addr)
}

// instanceAddr is per user so each signed-in user can run their own copy.
func instanceAddr() string {
	u, _ := user.Current()
	return `\\.\pipe\octelium-gui-` + u.Uid
}

// listenInstance fails if another copy already owns the pipe (winio creates
// the first pipe instance exclusively).
func listenInstance() (net.Listener, error) {
	return winio.ListenPipe(instanceAddr(), nil)
}

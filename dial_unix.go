//go:build !windows

package main

import (
	"context"
	"net"
	"os/user"
)

const defaultDaemonAddr = "/var/run/octelium/daemon.sock"

func dial(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", addr)
}

// instanceAddr is an abstract socket (leading @), so there is no file to
// clean up after a crash. Per user so each signed-in user can run their own copy.
func instanceAddr() string {
	u, _ := user.Current()
	return "@octelium-gui-" + u.Uid
}

func listenInstance() (net.Listener, error) {
	return net.Listen("unix", instanceAddr())
}

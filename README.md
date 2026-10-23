# octelium-gui
Octelium GUI for clients to easily connect and access resources.

Enter your domain, sign in in the browser, press **Connect**. Windows and Linux.

## How it works
The GUI does not reimplement Octelium. It is a thin client of `octelium daemon`, the
privileged daemon that ships in the official `octelium` CLI and owns the credentials,
the WireGuard device and DNS. The installer (which needs admin once) installs the CLI with
Octelium's own install script and registers the daemon as a service (`OcteliumDaemon` on
Windows, `octelium-daemon.service` on Linux). After that the GUI runs as a normal user
and talks to the daemon over `\\.\pipe\octelium-daemon` / `/var/run/octelium/daemon.sock`
using the gRPC API in `github.com/octelium/octelium/apis/client/daemonv1`. No admin
prompt when connecting.

The daemon binds to the first OS user that calls it; the installer launches the GUI as
the installing user so that user becomes the owner.

## Build
Needs Go and a C compiler (Fyne uses cgo). On Linux also `libgl1-mesa-dev xorg-dev`.

```bash
go test ./...
go build -ldflags "-H windowsgui" -o octelium-gui.exe .   # Windows
go build -o octelium-gui .                                # Linux
```

## Package
- Windows: `iscc installer/windows/octelium-gui.iss` (Inno Setup) → `dist/octelium-gui-setup-*.exe`.
  Runs `install.ps1` into `C:\Program Files\Octelium GUI\bin`, adds it to the system PATH,
  creates and starts the `OcteliumDaemon` service.
- Linux: `VERSION=0.1.0 nfpm pkg -f installer/linux/nfpm.yaml -p deb` (or `-p rpm`). The
  post-install runs `install.sh` (binary mode, without the official desktop app) and
  enables `octelium-daemon.service`.

## License
Apache-2.0, the same as the Octelium client. See `LICENSE` and `NOTICE`.

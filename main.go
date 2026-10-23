package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/octelium/octelium/apis/client/daemonv1"
	"google.golang.org/grpc/status"
)

type gui struct {
	app fyne.App
	win fyne.Window
	d   *daemon
	// Only touched on the UI thread.
	domain   string
	state    *daemonv1.DomainState
	signedIn bool // connect screen is showing

	// domain screen
	domainEntry *widget.Entry
	domainErr   *widget.Label
	// connect screen
	domainLabel, status, detail, connErr *widget.Label
	toggle                               *widget.Button
	// tray
	trayMenu   *fyne.Menu
	trayToggle *fyne.MenuItem
}

func main() {
	// --tray is used by the sign-in autostart: stay in the tray unless there is
	// nothing set up yet.
	tray := flag.Bool("tray", false, "start hidden in the system tray")
	flag.Parse()

	// One copy per user. A second launch asks the running copy to show its
	// window (unless it is the autostart) and exits.
	instance, err := listenInstance()
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if c, err := dial(ctx, instanceAddr()); err == nil {
			if !*tray {
				c.Write([]byte{1})
			}
			c.Close()
			return
		}
		// Not another copy, just no listener: run without the check.
	}

	a := app.NewWithID("com.octelium.gui")
	g := &gui{app: a, win: a.NewWindow("Octelium"), domain: a.Preferences().String("domain")}
	g.win.Resize(fyne.NewSize(340, 420))
	g.win.SetFixedSize(true)

	d, err := dialDaemon()
	if err != nil {
		// Only fails on a malformed target; a missing daemon surfaces in watch.
		panic(err)
	}
	g.d = d

	g.buildDomainScreen()
	g.buildConnectScreen()
	g.buildTray()
	g.showDomain()

	go d.watch(context.Background(), func() string {
		var s string
		fyne.DoAndWait(func() { s = g.domain })
		return s
	}, func(ds *daemonv1.DomainState, err error) { fyne.Do(func() { g.update(ds, err) }) })

	if instance != nil {
		go func() {
			for {
				c, err := instance.Accept()
				if err != nil {
					return
				}
				if n, _ := c.Read(make([]byte, 1)); n == 1 {
					fyne.Do(func() { g.win.Show(); g.win.RequestFocus() })
				}
				c.Close()
			}
		}()
	}

	if !*tray || g.domain == "" {
		g.win.Show()
	}
	a.Run()
}

func (g *gui) buildDomainScreen() {
	g.domainEntry = widget.NewEntry()
	g.domainEntry.SetPlaceHolder("example.com")
	g.domainEntry.SetText(g.domain)
	g.domainEntry.OnSubmitted = func(string) { g.signIn() }
	g.domainErr = widget.NewLabel("")
	g.domainErr.Importance = widget.DangerImportance
	g.domainErr.Wrapping = fyne.TextWrapWord
}

func (g *gui) showDomain() {
	g.signedIn = false
	cont := widget.NewButton("Continue", g.signIn)
	cont.Importance = widget.HighImportance
	g.win.SetContent(container.NewPadded(container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Octelium", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabel("Which domain?"),
			g.domainEntry,
			widget.NewLabel("Your cluster address, from your admin."),
			g.domainErr,
		), cont, nil, nil)))
}

func (g *gui) signIn() {
	domain := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(g.domainEntry.Text), "https://"), "/")
	if domain == "" {
		g.domainErr.SetText("Enter a domain.")
		return
	}
	g.domain = domain
	g.app.Preferences().SetString("domain", domain)
	g.domainErr.SetText("")

	ctx, cancel := context.WithCancel(context.Background())
	var loginURL string
	reopen := widget.NewButton("Reopen browser", func() { g.openURL(loginURL) })
	waiting := widget.NewLabel("Starting sign-in…")
	g.win.SetContent(container.NewPadded(container.NewBorder(
		widget.NewLabel(domain), container.NewGridWithColumns(2, reopen, widget.NewButton("Cancel", cancel)), nil, nil,
		container.NewCenter(waiting))))

	go func() {
		err := g.d.run(ctx, func(ctx context.Context) (*daemonv1.Operation, error) {
			return g.d.Authenticate(ctx, &daemonv1.AuthenticateRequest{
				Domain: domain,
				Type:   &daemonv1.AuthenticateRequest_Browser_{Browser: &daemonv1.AuthenticateRequest_Browser{}},
			})
		}, func(u string) {
			fyne.Do(func() {
				loginURL = u
				waiting.SetText("Finish signing in in your browser.")
				g.openURL(u)
			})
		})
		fyne.Do(func() {
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					g.domainErr.SetText("Sign-in failed: " + err.Error())
				}
				g.showDomain()
				return
			}
			g.showConnect()
		})
	}()
}

// openURL is safe to call from any goroutine.
func (g *gui) openURL(u string) {
	if parsed, err := url.Parse(u); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
		fyne.Do(func() { g.app.OpenURL(parsed) })
	}
}

func (g *gui) buildConnectScreen() {
	g.domainLabel = widget.NewLabel("")
	g.status = widget.NewLabelWithStyle("Disconnected", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	g.detail = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})
	g.connErr = widget.NewLabel("")
	g.connErr.Importance = widget.DangerImportance
	g.connErr.Wrapping = fyne.TextWrapWord
	g.toggle = widget.NewButton("Connect", g.toggleConnection)
	g.toggle.Importance = widget.HighImportance
}

func (g *gui) showConnect() {
	g.signedIn = true
	g.domainLabel.SetText(g.domain)
	signOut := widget.NewButton("Sign out", g.signOut)
	signOut.Importance = widget.LowImportance
	g.win.SetContent(container.NewPadded(container.NewBorder(
		g.domainLabel,
		container.NewVBox(g.connErr, container.NewBorder(nil, nil, nil, signOut)),
		nil, nil,
		container.NewCenter(container.NewVBox(
			container.NewGridWrap(fyne.NewSize(160, 160), g.toggle),
			g.status, g.detail)))))
	g.render()
}

func (g *gui) connected() bool {
	s := g.state.GetConnection().GetState()
	return s == daemonv1.ConnectionStatus_CONNECTED || s == daemonv1.ConnectionStatus_RECONNECTING
}

func (g *gui) toggleConnection() {
	domain, disconnect := g.domain, g.connected()
	g.connErr.SetText("")
	g.toggle.Disable()
	go func() {
		err := g.d.run(context.Background(), func(ctx context.Context) (*daemonv1.Operation, error) {
			if disconnect {
				return g.d.Disconnect(ctx, &daemonv1.DisconnectRequest{Domain: domain})
			}
			// Same as `octelium connect --ip-mode both`.
			return g.d.Connect(ctx, &daemonv1.ConnectRequest{Domain: domain,
				Options: &daemonv1.ConnectionOptions{L3Mode: daemonv1.ConnectionOptions_BOTH}})
		}, g.openURL)
		fyne.Do(func() {
			g.toggle.Enable()
			if err != nil {
				g.connErr.SetText(err.Error())
			}
		})
	}()
}

func (g *gui) signOut() {
	domain := g.domain
	go func() {
		err := g.d.run(context.Background(), func(ctx context.Context) (*daemonv1.Operation, error) {
			return g.d.Logout(ctx, &daemonv1.LogoutRequest{Domain: domain})
		}, g.openURL)
		fyne.Do(func() {
			if err != nil {
				g.connErr.SetText("Sign out failed: " + err.Error())
				return
			}
			g.showDomain()
		})
	}()
}

const unreachable = "Can't reach the Octelium service. Make sure it's installed and running."

// update applies a daemon status push. It also moves between the domain and
// connect screens when the daemon's view of the sign-in changes.
func (g *gui) update(ds *daemonv1.DomainState, err error) {
	g.state = ds
	if err != nil {
		msg := unreachable + "\n\n" + status.Convert(err).Message()
		g.domainErr.SetText(msg)
		g.connErr.SetText(msg)
		g.render()
		return
	}
	if strings.HasPrefix(g.domainErr.Text, unreachable) {
		g.domainErr.SetText("")
		g.connErr.SetText("")
	}

	switch ds.GetAuthentication().GetState() {
	case daemonv1.AuthenticationStatus_AUTHENTICATED:
		if !g.signedIn {
			g.showConnect()
		}
	case daemonv1.AuthenticationStatus_LOGGED_OUT:
		if g.signedIn {
			g.domainErr.SetText("Your session ended. Sign in again.")
			g.showDomain()
		}
	}
	g.render()
}

func (g *gui) render() {
	conn := g.state.GetConnection()
	state := conn.GetState()
	g.status.SetText(map[daemonv1.ConnectionStatus_State]string{
		daemonv1.ConnectionStatus_CONNECTING:    "Connecting…",
		daemonv1.ConnectionStatus_CONNECTED:     "Connected",
		daemonv1.ConnectionStatus_RECONNECTING:  "Reconnecting…",
		daemonv1.ConnectionStatus_DISCONNECTING: "Disconnecting…",
	}[state])
	if g.status.Text == "" {
		g.status.SetText("Disconnected")
	}

	g.detail.SetText("")
	if g.connected() {
		var ips []string
		for _, a := range conn.GetAddresses() {
			if a.GetV4() != "" {
				ips = append(ips, strings.Split(a.GetV4(), "/")[0])
			}
		}
		detail := strings.Join(ips, ", ")
		if at := conn.GetConnectedAt(); at != nil {
			detail = fmt.Sprintf("Up %s · %s", time.Since(at.AsTime()).Round(time.Second), detail)
		}
		g.detail.SetText(detail)
	}
	if msg := g.state.GetLastError().GetMessage(); msg != "" && g.connErr.Text == "" {
		g.connErr.SetText(msg)
	}

	label, imp, icon := "Connect", widget.HighImportance, fyne.Resource(theme.NewDisabledResource(theme.RadioButtonIcon()))
	if g.connected() {
		label, imp, icon = "Disconnect", widget.DangerImportance, theme.NewSuccessThemedResource(theme.RadioButtonCheckedIcon())
	}
	g.toggle.SetText(label)
	g.toggle.Importance = imp
	g.toggle.Refresh()
	if g.trayMenu != nil {
		g.trayToggle.Label = label
		g.trayMenu.Refresh()
		g.app.(desktop.App).SetSystemTrayIcon(icon)
	}
}

func (g *gui) buildTray() {
	desk, ok := g.app.(desktop.App)
	if !ok {
		return
	}
	g.trayToggle = fyne.NewMenuItem("Connect", func() {
		if !g.signedIn {
			g.win.Show()
			return
		}
		g.toggleConnection()
	})
	g.trayMenu = fyne.NewMenu("Octelium", g.trayToggle, fyne.NewMenuItem("Open", g.win.Show))
	desk.SetSystemTrayMenu(g.trayMenu)
	desk.SetSystemTrayIcon(theme.NewDisabledResource(theme.RadioButtonIcon()))
	g.win.SetCloseIntercept(g.win.Hide) // keep running in the tray; Quit is in the tray menu
}

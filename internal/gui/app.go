/*
Package gui is the desktop front end (spec 001 R6).

The window renders internal/core and does nothing else: it holds no switching
logic of its own and reaches no further than the core. Core is headless so that
every operation can be tested without a display, and so that --status and the
hotkey flags can run the same code the window runs.

The window itself (navigation, content pane, status bar, busy indicator and
result banners) is fynedesygn's shell; this package supplies the sections and
the status bar's segments. Every core call runs off the UI thread and hops back
with fyne.Do. Nothing transient reflows the interface: result banners and the
progress indicator float over the content as popups.
*/
package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/devices"
	"github.com/ushineko/ototo/internal/instance"
	"github.com/ushineko/ototo/internal/loopback"
	"github.com/ushineko/ototo/internal/notify"
)

// appID names the preference store and, on Wayland, the window's app_id,
// which the compositor matches to the desktop entry of the same basename.
const appID = "io.ushineko.ototo"

// ui holds the program's state and the shell that draws it.
type ui struct {
	// sh is the window. The shell stores itself here through Options.OnCreate
	// before it builds the first section, so every builder can rely on it.
	sh      *shell.Shell
	version string
	commit  string
	// configPath and server are the --config and --server overrides, carried
	// into every core request so that the window and `ototo --status` read the
	// same document and the same server.
	configPath string
	server     string
	// sw is the switching state (the breaker, the last switch), held for the
	// life of the process; loop is the 5 s tick that drives it.
	sw   *core.Switcher
	loop ticker
	// hiddenToTray says the window is hidden rather than closed.
	hiddenToTray bool
	// selected is the Outputs row the user picked, -1 for none. It is what
	// enables the row actions, and it survives a rebuild.
	selected int

	// Loaded from the core on a goroutine, read and written on the UI thread.
	//
	// The result has a companion flag rather than being tested for emptiness:
	// "loaded and empty" and "not loaded" are different states. loading is set
	// before the read starts, so a rebuild during the read does not start a
	// second one.
	status   core.StatusResult
	statusOK bool
	loading  bool
	// loopback is the line-in loopback's state, read with the status.
	loopback loopback.State
	// desktop is what is installed into the desktop, read with the status.
	desktop core.DesktopState
	// osd is the volume indicator; nil until the window exists. stopWatch
	// ends the subscription that feeds it.
	osd       *indicator
	stopWatch context.CancelFunc
	// live is what the Outputs section updates in place when only the
	// volume changed: rebuilding the section for a volume step made the
	// slider jump under the pointer.
	live liveOutputs
}

// sectionTitles is the navigation in order.
//
// The order and the names live here, apart from the icons, because a theme icon
// cannot be constructed before an app exists: the flag help lists these while
// parsing flags. A title with no builder draws nothing, so the two are kept in
// step by TestSectionNamesNeedsNoApp rather than by memory.
var sectionTitles = []string{
	"Outputs", "Microphone", "Settings", "Appearance", "About",
}

// sectionEntry is what a section is made of: a deferred icon, its builder,
// and the hook that runs when the user navigates to it.
type sectionEntry struct {
	icon   func() fyne.Resource
	build  func(*ui) fyne.CanvasObject
	arrive func(*ui)
}

// sectionBuilders is what each section is made of. A function rather than a
// package variable: the builders reach back to the sections when a section
// rebuilds itself, and Go reports that as an initialization cycle in a
// package-level map.
func sectionBuilders() map[string]sectionEntry {
	return map[string]sectionEntry{
		// The sound server is written to by every other program on the
		// machine, so Outputs reads it again on arrival: the hook and not the
		// builder, or the end of the read would rebuild the section that
		// started it, which would read again.
		"Outputs":    {theme.VolumeUpIcon, (*ui).buildOutputs, (*ui).loadStatus},
		"Microphone": {theme.MediaRecordIcon, (*ui).buildMicrophone, nil},
		"Settings":   {theme.SettingsIcon, (*ui).buildSettings, nil},
		"Appearance": {theme.ColorPaletteIcon, (*ui).buildAppearance, nil},
		"About":      {theme.HelpIcon, (*ui).buildAbout, nil},
	}
}

// sections is the navigation as the shell takes it. A nil u is enough for the
// titles, which is all SectionNames needs.
func sections(u *ui) []shell.Section {
	builders := sectionBuilders()
	out := make([]shell.Section, 0, len(sectionTitles))
	for _, title := range sectionTitles {
		b, ok := builders[title]
		if !ok {
			continue // a title with no builder draws nothing; see SectionNames
		}
		sec := shell.NewSection(title, b.icon, func(*shell.Shell) fyne.CanvasObject { return b.build(u) })
		if b.arrive != nil {
			sec.OnArrive(func() { b.arrive(u) })
		}
		out = append(out, sec)
	}
	return out
}

// SectionNames lists the navigation entries, for --section and for a capture
// script to iterate. Reads the titles rather than building the sections: this
// is called while parsing flags, before there is an app to hang an icon on.
func SectionNames() []string { return shell.Names(sections(nil)) }

// SchemeNames lists the colour schemes, for the same reason.
func SchemeNames() []string { return fdtheme.SchemeNames() }

// Options configure a run. Section and Scheme exist so a capture script can
// deep-link into the window. They override the saved appearance for that run
// without saving over it.
type Options struct {
	Version string
	Commit  string
	// ConfigPath is the --config override; empty uses the default document.
	ConfigPath string
	// Server is the --server override; empty uses the session's sound server.
	Server  string
	Section string // navigation entry to open on; empty means the first
	Scheme  string // color scheme to force; empty means the saved one
}

/*
Run opens the window and blocks until it is closed, and returns the exit
code. When another ototo is already running, it asks that one to show its
window and returns without opening a second (D10).
*/
func Run(o Options) int {
	u := newUI(o)
	srv, err := instance.Listen(u.handle)
	if errors.Is(err, instance.ErrRunning) {
		if _, ok := instance.Ask("show"); ok {
			fmt.Fprintln(os.Stderr, "ototo is already running; showing its window")
			return 0
		}
		fmt.Fprintln(os.Stderr, "ototo: another ototo holds the lock but does not answer; is it starting up?")
		return 1
	}
	if err != nil {
		// Without the listener a second launch does nothing and a hotkey
		// acts on its own; the window still works.
		fmt.Fprintln(os.Stderr, "ototo: single-instance listener unavailable:", err)
	} else {
		defer srv.Close()
	}
	router := routingNotifier{u: u}
	if bus, err := notify.Session(); err == nil {
		router.bus = bus
	} else {
		fmt.Fprintln(os.Stderr, "ototo: notifications unavailable:", err)
	}
	u.sw.Notifier = router
	// A TERM or an INT ends the program the way Quit does, with the tick,
	// the watcher and the loopback child stopped. Without this the tray's
	// signal handling swallows TERM and the process has to be killed.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		fyne.Do(u.quit)
	}()
	shell.Run(u.shellOptions(o))
	return 0
}

func newUI(o Options) *ui {
	return &ui{
		version: o.Version, commit: o.Commit, configPath: o.ConfigPath, server: o.Server,
		sw: core.NewSwitcher(), selected: -1,
	}
}

// shellOptions describes this program to the shell.
//
// The preference store the app ID names holds the appearance settings and
// nothing else: every setting that decides behaviour lives in config.json, so
// that `ototo --status` in a terminal and this window agree.
func (u *ui) shellOptions(o Options) shell.Options {
	return shell.Options{
		AppID:     appID,
		Name:      "ototo",
		Version:   u.version,
		Icon:      appIcon(),
		Sections:  sections(u),
		Section:   o.Section,
		Scheme:    o.Scheme,
		StatusBar: func(*shell.Shell) []fyne.CanvasObject { return u.statusSegments() },
		OnCreate:  func(s *shell.Shell) { u.sh = s },
		// The status bar names the server and the default output from every
		// section, so the read starts here rather than being left to Outputs.
		// The tray, the close intercept and the tick belong to the window,
		// so they start here too.
		OnStart: func(s *shell.Shell) {
			s.Window.SetCloseIntercept(u.onClose)
			u.setupTray()
			u.osd = newIndicator(s.App)
			ctx, cancel := context.WithCancel(context.Background())
			u.stopWatch = cancel
			go u.watchVolume(ctx)
			u.restoreLoopback()
			u.loadStatus()
			u.loop.start(u)
		},
		OnStop: func(*shell.Shell) {
			u.shutdown()
		},
		OnInvalidate: u.onInvalidate,

		// The navigation's shape is the user's: titles with icons, icons
		// alone, or hidden entirely, down the left or along the top. One
		// stock control in the header offers exactly what is listed here, and
		// Ctrl+B hides and restores. The choice is stored by the library
		// under fynedesygn.nav, so it outlives the run.
		NavModes:      []shell.NavMode{shell.NavLabels, shell.NavIcons, shell.NavHidden},
		NavPlacements: []shell.NavPlacement{shell.NavLeft, shell.NavTop},
	}
}

// request is the core request every operation from this window carries.
func (u *ui) request() core.Request {
	return core.Request{ConfigPath: u.configPath, Server: u.server, Events: u.events()}
}

// events routes the core's log lines to stderr; debug lines too when
// OTOTO_DEBUG is set, which is how a report of "nothing happened" is read.
func (u *ui) events() core.Events {
	return core.Events{Log: func(level core.Level, msg string) {
		if level >= core.LevelInfo || os.Getenv("OTOTO_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "ototo: %s: %s\n", level, msg)
		}
	}}
}

// loadStatus reads what the machine has, through the shell's loader so the
// busy indicator appears if the server is slow to answer. One read at a time:
// a rebuild during the read must not start a second one.
func (u *ui) loadStatus() {
	if u.loading {
		return
	}
	u.loading = true
	u.sh.Load("Reading the sound server...", func(ctx context.Context) error {
		res, err := core.Status(ctx, core.StatusRequest{Request: u.request()})
		var lb loopback.State
		if err == nil && res.ServerError == "" {
			// Best effort: the card says "unknown" rather than the whole
			// status failing over a systemctl that did not answer.
			lb, _ = u.sw.LoopbackState(ctx, core.LoopbackRequest{Request: u.request()})
		}
		dt := core.DesktopStatus(ctx)
		fyne.Do(func() {
			u.loading = false
			if err == nil {
				u.status = res
				u.loopback = lb
				u.desktop = dt
				u.statusOK = true
			}
		})
		return err
	})
}

/*
refreshQuietly reads the state again off the UI thread and redraws the
section only when something changed. It is what the tick and the hotkey
path use: a read every five seconds through the shell's loader would
rebuild the section twice a tick, once when the work starts and once when
it ends, and the person would see the list flash while reading it.
*/
func (u *ui) refreshQuietly() {
	ctx, cancel := context.WithTimeout(context.Background(), TickInterval*2)
	defer cancel()
	res, err := core.Status(ctx, core.StatusRequest{Request: u.request()})
	if err != nil {
		return
	}
	var lb loopback.State
	if res.ServerError == "" {
		lb, _ = u.sw.LoopbackState(ctx, core.LoopbackRequest{Request: u.request()})
	}
	dt := core.DesktopStatus(ctx)
	fyne.Do(func() {
		if u.loading {
			return // a loader is reading; its result is newer than this one
		}
		changed := !u.statusOK || !reflect.DeepEqual(res, u.status) ||
			!reflect.DeepEqual(lb, u.loopback) || !reflect.DeepEqual(dt, u.desktop)
		volumeOnly := changed && u.statusOK && reflect.DeepEqual(lb, u.loopback) && reflect.DeepEqual(dt, u.desktop) &&
			sameExceptVolume(res, u.status)
		u.status, u.loopback, u.desktop = res, lb, dt
		u.statusOK = true
		switch {
		case volumeOnly:
			u.updateVolumeInPlace()
		case changed:
			u.sh.Refresh()
			u.sh.RedrawStatus()
		}
	})
}

// sameExceptVolume says two readings differ only in a level or a mute.
func sameExceptVolume(a, b core.StatusResult) bool {
	flat := func(r core.StatusResult) core.StatusResult {
		r.Devices = append([]devices.Device{}, r.Devices...)
		for i := range r.Devices {
			r.Devices[i].Volume, r.Devices[i].Mute = 0, false
		}
		return r
	}
	return reflect.DeepEqual(flat(a), flat(b))
}

// onInvalidate discards what was loaded from the core, which makes the
// sections fetch again. Off screen, clearing the flag is the whole of the
// work: fetching anyway would send a headless test off to the real server.
func (u *ui) onInvalidate(s *shell.Shell) {
	u.statusOK = false
	if !s.OnScreen() {
		return
	}
	u.loadStatus()
}

// statusSegments is the sound server and the default output, from every
// section.
func (u *ui) statusSegments() []fyne.CanvasObject {
	server := widgets.StatusText("reading…", fd.StatusInfo)
	output := widgets.StatusText("—", fd.StatusInfo)
	if u.statusOK {
		if u.status.ServerError != "" {
			server = widgets.StatusText("not reached", fd.StatusBad)
			output = widgets.StatusText("none", fd.StatusBad)
		} else {
			server = widgets.StatusText(u.status.Server.Name+" "+u.status.Server.Version, fd.StatusGood)
			output = widgets.StatusText(defaultOutputText(u.status), fd.StatusGood)
		}
	}
	return []fyne.CanvasObject{
		widgets.Dim("server"), server, widgets.Sep(),
		widgets.Dim("output"), output,
	}
}

// defaultOutputText names the default output by its description, which is
// what a person calls it, falling back to the sink name.
func defaultOutputText(res core.StatusResult) string {
	for _, d := range res.Devices {
		if d.Default {
			return d.Name
		}
	}
	// The default sink can be one the list hides, which today is only the
	// JamesDSP sink; the routing target replaces this line with R6.
	return widgets.OrNone(res.Server.DefaultSink, "none")
}

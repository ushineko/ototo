package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"
	"github.com/ushineko/fynedesygn/glance/kwin"
)

// IndicatorRule is the KWin rule the indicator needs (spec 001 R8.5): no
// titlebar, above other windows, out of the taskbar, the switcher and the
// pager, and never the focus. It matches the app ID and the indicator's
// title, so the main window keeps its titlebar.
func IndicatorRule() kwin.Rule {
	return kwin.Rule{
		AppID:        AppID,
		Title:        IndicatorTitle,
		Description:  "ototo volume indicator",
		AlwaysOnTop:  true,
		NoBorder:     true,
		SkipTaskbar:  true,
		SkipSwitcher: true,
		SkipPager:    true,
		NoFocus:      true,
	}
}

// RuleInstalled reports whether the indicator's rule is in kwinrulesrc.
func RuleInstalled() (bool, error) {
	_, ok, err := kwin.LookupTitled(AppID, IndicatorTitle)
	return ok, err //nolint:wrapcheck // the library names the file
}

// InstallRule writes the rule and asks KWin to read its rules again. With
// no KWin on the bus, the rule is written and the reload is reported.
func InstallRule(ctx context.Context) error {
	if err := kwin.Install(IndicatorRule()); err != nil {
		return err //nolint:wrapcheck // the library names the file
	}
	return reconfigure(ctx)
}

// RemoveRule takes the rule out and asks KWin to read its rules again.
func RemoveRule(ctx context.Context) (bool, error) {
	removed, err := kwin.RemoveTitled(AppID, IndicatorTitle)
	if err != nil || !removed {
		return removed, err //nolint:wrapcheck // the library names the file
	}
	return true, reconfigure(ctx)
}

func reconfigure(ctx context.Context) error {
	call := kwin.ReconfigureCall()
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("the rule is written, but KWin was not told to reload it: %w", err)
	}
	obj := conn.Object(call.Destination, dbus.ObjectPath(call.Path))
	if err := obj.CallWithContext(ctx, call.Interface+"."+call.Method, 0).Err; err != nil {
		return fmt.Errorf("the rule is written, but KWin was not told to reload it: %w", err)
	}
	return nil
}

// KWin's scripting service on the session bus.
const (
	kwinName       = "org.kde.KWin"
	scriptingPath  = dbus.ObjectPath("/Scripting")
	scriptingIface = "org.kde.kwin.Scripting"
	scriptIface    = "org.kde.kwin.Script"
	placeScript    = "ototo-indicator-place"
)

/*
placeScriptFor is the KWin script that puts ototo's own indicator window in
the middle of the screen the pointer is on. It finds the window by this
program's class and the indicator's title and touches nothing else.

Fyne cannot place a window on Wayland, and a rule's position is fixed to one
screen; the compositor knows where the pointer is and moves its own windows.
*/
func placeScriptFor(class, title string) string {
	c, _ := json.Marshal(class)
	t, _ := json.Marshal(title)
	return `const cls = ` + string(c) + `;
const title = ` + string(t) + `;
const pos = workspace.cursorPos;
let screen = null;
for (const s of workspace.screens) {
    const g = s.geometry;
    if (pos.x >= g.x && pos.x < g.x + g.width && pos.y >= g.y && pos.y < g.y + g.height) {
        screen = g;
        break;
    }
}
if (screen !== null) {
    for (const w of workspace.windowList()) {
        if (w.resourceClass !== cls || w.caption !== title) {
            continue;
        }
        const f = w.frameGeometry;
        w.frameGeometry = {
            x: screen.x + Math.round((screen.width - f.width) / 2),
            y: screen.y + Math.round((screen.height - f.height) / 2),
            width: f.width,
            height: f.height,
        };
    }
}
`
}

/*
PlaceIndicator moves the indicator window to the middle of the screen the
pointer is on, through KWin's scripting service: the script is loaded, run
once and unloaded. Without KWin the call fails and the window stays where
the compositor put it, which the caller treats as normal.
*/
func PlaceIndicator(ctx context.Context) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("connect to the session bus: %w", err)
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	// The runtime directory is the user's own, as for the instance socket;
	// the file name is this package's constant.
	path := filepath.Join(dir, placeScript+".js")
	if err := os.WriteFile(path, []byte(placeScriptFor(AppID, IndicatorTitle)), 0o600); err != nil { //nolint:gosec // the user's runtime directory, a fixed name
		return fmt.Errorf("write the placement script: %w", err)
	}
	defer func() { _ = os.Remove(path) }() //nolint:gosec // the file this function wrote

	scripting := conn.Object(kwinName, scriptingPath)
	// A script of this name left loaded by a run that was interrupted
	// would make loadScript answer with its id and never run it again.
	_ = scripting.CallWithContext(ctx, scriptingIface+".unloadScript", 0, placeScript).Err
	var id int32
	if err := scripting.CallWithContext(ctx, scriptingIface+".loadScript", 0, path, placeScript).Store(&id); err != nil {
		return fmt.Errorf("load the placement script into KWin: %w", err)
	}
	defer func() { _ = scripting.CallWithContext(ctx, scriptingIface+".unloadScript", 0, placeScript).Err }()
	obj := conn.Object(kwinName, dbus.ObjectPath(fmt.Sprintf("/Scripting/Script%d", id)))
	if err := obj.CallWithContext(ctx, scriptIface+".run", 0).Err; err != nil {
		return fmt.Errorf("run the placement script: %w", err)
	}
	return nil
}

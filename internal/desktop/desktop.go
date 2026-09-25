/*
Package desktop is what ototo installs into the desktop on request, and
takes out again (spec 001 D9): the autostart entry and the indicator's
window rule. The volume keys (R11) join them.

Every step here is explicit and reversible. Nothing in this package runs at
first start; the Settings section and --desktop are the callers, and each
one says what it changes and how it is undone.
*/
package desktop

import (
	"fmt"
	"os"
	"path/filepath"
)

// AppID is the application's unique ID, which is also the desktop entry's
// basename and the window class KWin matches.
const AppID = "io.ushineko.ototo"

// IndicatorTitle is the indicator window's title, which its KWin rule
// matches. The main window shares the app ID, so the rule keys on this too.
const IndicatorTitle = "ototo-indicator"

// autostartEntry is the desktop entry written under autostart. It runs the
// binary on PATH; the launcher entry the installer places is the same.
const autostartEntry = `[Desktop Entry]
Type=Application
Name=ototo
Comment=Switch audio outputs and Bluetooth headsets by priority
Exec=ototo
Icon=ototo
Terminal=false
NoDisplay=false
X-KDE-autostart-after=panel
StartupNotify=false
`

// AutostartPath is the entry's path: $XDG_CONFIG_HOME/autostart/<AppID>.desktop.
func AutostartPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find the home directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", AppID+".desktop"), nil
}

// AutostartInstalled reports whether the entry exists.
func AutostartInstalled() (bool, error) {
	path, err := AutostartPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look for %s: %w", path, err)
	}
	return true, nil
}

// InstallAutostart writes the entry, replacing one that is there.
func InstallAutostart() error {
	path, err := AutostartPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(autostartEntry), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// RemoveAutostart deletes the entry and says whether there was one.
func RemoveAutostart() (bool, error) {
	path, err := AutostartPath()
	if err != nil {
		return false, err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("remove %s: %w", path, err)
	}
	return true, nil
}

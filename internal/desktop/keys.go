package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/godbus/dbus/v5"
)

/*
The volume keys (spec 001 R11).

Plasma binds Volume Up and Volume Down to its own mixer through kglobalaccel.
The original freed them by hand-editing the shortcuts file and left the user
to create two custom shortcuts in System Settings. Here the program does the
whole thing: it records what holds the keys, releases it, and registers two
command shortcuts of its own; turned off, it unregisters them and gives the
keys back to what held them.

Everything goes through org.kde.kglobalaccel on the session bus, never by
editing the file, which kglobalaccel keeps in memory and writes itself. The
registration must come from one connection that stays open until the keys
are set; godbus shares one connection per process, which is that.
*/

// Qt key codes, as kglobalaccel takes them. The enum goes VolumeDown,
// VolumeMute, VolumeUp; the first registration of this program had them
// one off and the shortcuts file named the keys it had really bound.
const (
	keyVolumeDown int32 = 0x01000070
	keyVolumeUp   int32 = 0x01000072
)

// The two command shortcuts, as desktop entries under applications/.
const (
	upEntry   = "io.ushineko.ototo.vol-up.desktop"
	downEntry = "io.ushineko.ototo.vol-down.desktop"
	// launch is the action a command shortcut has.
	launch = "_launch"
	// recordName holds what held the keys, in ototo's config directory.
	recordName = "volume-keys.json"
)

const (
	accelName  = "org.kde.kglobalaccel"
	accelPath  = dbus.ObjectPath("/kglobalaccel")
	accelIface = "org.kde.KGlobalAccel"
	// setPresent makes a registration take; noAutoloading makes the keys
	// given the keys used, where without it kglobalaccel keeps the keys it
	// already stored for an action it knows and reports those back. An
	// install is the person's explicit choice, so the keys given are the
	// keys wanted, on the second install as much as the first.
	setPresent    uint32 = 2
	noAutoloading uint32 = 4
)

// keySeq is one key sequence on the wire: a(ai) is a struct holding an
// array.
type keySeq struct{ Keys []int32 }

// Holder is a shortcut that held one of the keys before ototo took it.
type Holder struct {
	// Component and Action name the shortcut to kglobalaccel: a desktop
	// file and "_launch" for a command shortcut, "kmix" and
	// "increase_volume" for the mixer's.
	Component string `json:"component"`
	Action    string `json:"action"`
	// Key is the key it held.
	Key int32 `json:"key"`
	// Service says the holder is a command shortcut (a desktop entry),
	// which is released by unregistering and restored by registering; a
	// foreign component's keys are set through setForeignShortcutKeys.
	Service bool `json:"service"`
}

// record is what RemoveVolumeKeys gives back.
type record struct {
	Holders []Holder `json:"holders"`
}

func recordPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find the home directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "ototo", recordName), nil
}

// ShortcutsPath is kglobalshortcutsrc, read to find what holds the keys.
func ShortcutsPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find the home directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "kglobalshortcutsrc"), nil
}

// VolumeKeysInstalled reports whether ototo holds the keys: the record of
// what it took exists.
func VolumeKeysInstalled() (bool, error) {
	path, err := recordPath()
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

func commandEntry(name, exec string) string {
	return "[Desktop Entry]\nType=Application\nName=" + name + "\nExec=" + exec +
		"\nNoDisplay=true\nStartupNotify=false\nX-KDE-GlobalAccel-CommandShortcut=true\n"
}

func applicationsDir() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find the home directory: %w", err)
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "applications"), nil
}

/*
holders reads the shortcuts file and returns what holds Volume Up and
Volume Down: a command shortcut (a `[services][x.desktop]` section whose
`_launch` is the key) or another component's action whose current sequence
is the key. A value is `current,default,friendly`, and current can be several
sequences separated by a tab.
*/
func holders(text string) []Holder {
	var out []Holder
	section, service := "", false
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			service = false
			if rest, ok := strings.CutPrefix(line, "[services]["); ok {
				section = strings.TrimSuffix(rest, "]")
				service = true
			} else {
				section = strings.Trim(line, "[]")
			}
			continue
		}
		if section == "" || section == "$Version" || strings.HasPrefix(line, "_k_friendly_name") {
			continue
		}
		action, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		current := value
		if !service {
			current, _, _ = strings.Cut(value, ",")
		}
		for _, seq := range strings.Split(current, "\t") {
			var key int32
			switch strings.TrimSpace(seq) {
			case "Volume Up":
				key = keyVolumeUp
			case "Volume Down":
				key = keyVolumeDown
			default:
				continue
			}
			if service && section == upEntry || service && section == downEntry {
				continue // ototo's own
			}
			out = append(out, Holder{Component: section, Action: action, Key: key, Service: service})
		}
	}
	return out
}

func accel(conn *dbus.Conn) dbus.BusObject { return conn.Object(accelName, accelPath) }

func actionID(component, action, friendly string) []string {
	return []string{component, action, friendly, "Launch"}
}

/*
InstallVolumeKeys makes Volume Up and Volume Down run ototo (R11.1). What
held them is recorded first, once: a second install does not overwrite the
record with ototo's own entries.
*/
func InstallVolumeKeys(ctx context.Context) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("connect to the session bus: %w", err)
	}
	apps, err := applicationsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(apps, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", apps, err)
	}
	for _, e := range []struct{ file, name, exec string }{
		{upEntry, "ototo: volume up", "ototo --vol-up"},
		{downEntry, "ototo: volume down", "ototo --vol-down"},
	} {
		if err := os.WriteFile(filepath.Join(apps, e.file), []byte(commandEntry(e.name, e.exec)), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", e.file, err)
		}
	}

	// What holds the keys, recorded before it is released.
	recPath, err := recordPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(recPath); os.IsNotExist(err) {
		shortcuts, err := ShortcutsPath()
		if err != nil {
			return err
		}
		text, err := os.ReadFile(shortcuts) //nolint:gosec // the user's own config
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", shortcuts, err)
		}
		rec := record{Holders: holders(string(text))}
		if err := os.MkdirAll(filepath.Dir(recPath), 0o750); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(recPath), err)
		}
		raw, _ := json.MarshalIndent(rec, "", "  ")
		if err := os.WriteFile(recPath, raw, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", recPath, err)
		}
		for _, h := range rec.Holders {
			if err := release(ctx, conn, h); err != nil {
				return err
			}
		}
	}

	for _, e := range []struct {
		file, name string
		key        int32
	}{
		{upEntry, "ototo: volume up", keyVolumeUp},
		{downEntry, "ototo: volume down", keyVolumeDown},
	} {
		if err := register(ctx, conn, actionID(e.file, launch, e.name), e.key); err != nil {
			return err
		}
	}
	return nil
}

func release(ctx context.Context, conn *dbus.Conn, h Holder) error {
	if h.Service {
		var ok bool
		if err := accel(conn).CallWithContext(ctx, accelIface+".unregister", 0, h.Component, h.Action).Store(&ok); err != nil {
			return fmt.Errorf("release %s from %s: %w", keyName(h.Key), h.Component, err)
		}
		return nil
	}
	call := accel(conn).CallWithContext(ctx, accelIface+".setForeignShortcutKeys", 0,
		actionID(h.Component, h.Action, ""), []keySeq{})
	if call.Err != nil {
		return fmt.Errorf("release %s from %s: %w", keyName(h.Key), h.Component, call.Err)
	}
	return nil
}

func register(ctx context.Context, conn *dbus.Conn, id []string, key int32) error {
	if err := accel(conn).CallWithContext(ctx, accelIface+".doRegister", 0, id).Err; err != nil {
		return fmt.Errorf("register %s: %w", id[0], err)
	}
	var got []keySeq
	if err := accel(conn).CallWithContext(ctx, accelIface+".setShortcutKeys", 0, id, []keySeq{{Keys: []int32{key}}}, setPresent|noAutoloading).Store(&got); err != nil {
		return fmt.Errorf("bind %s to %s: %w", keyName(key), id[0], err)
	}
	if len(got) == 0 || len(got[0].Keys) == 0 || got[0].Keys[0] != key {
		return fmt.Errorf("%s is held by another shortcut; release it in System Settings and turn this on again", keyName(key))
	}
	return nil
}

// RemoveVolumeKeys unregisters ototo's shortcuts, removes their entries,
// and gives the keys back to what held them. It says whether there was
// anything to remove.
func RemoveVolumeKeys(ctx context.Context) (bool, error) {
	recPath, err := recordPath()
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(recPath) //nolint:gosec // ototo's own config directory
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", recPath, err)
	}
	var rec record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return false, fmt.Errorf("parse %s: %w", recPath, err)
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return false, fmt.Errorf("connect to the session bus: %w", err)
	}
	var errs []error
	for _, file := range []string{upEntry, downEntry} {
		var ok bool
		if err := accel(conn).CallWithContext(ctx, accelIface+".unregister", 0, file, launch).Store(&ok); err != nil {
			errs = append(errs, fmt.Errorf("unregister %s: %w", file, err))
		}
	}
	apps, err := applicationsDir()
	if err == nil {
		for _, file := range []string{upEntry, downEntry} {
			if err := os.Remove(filepath.Join(apps, file)); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("remove %s: %w", file, err))
			}
		}
	}
	for _, h := range rec.Holders {
		if h.Service {
			if err := register(ctx, conn, actionID(h.Component, h.Action, h.Component), h.Key); err != nil {
				errs = append(errs, fmt.Errorf("give back: %w", err))
			}
			continue
		}
		call := accel(conn).CallWithContext(ctx, accelIface+".setForeignShortcutKeys", 0,
			actionID(h.Component, h.Action, ""), []keySeq{{Keys: []int32{h.Key}}})
		if call.Err != nil {
			errs = append(errs, fmt.Errorf("give %s back to %s: %w", keyName(h.Key), h.Component, call.Err))
		}
	}
	if err := os.Remove(recPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("remove %s: %w", recPath, err))
	}
	return true, errors.Join(errs...)
}

func keyName(key int32) string {
	switch key {
	case keyVolumeUp:
		return "Volume Up"
	case keyVolumeDown:
		return "Volume Down"
	default:
		return fmt.Sprintf("key %#x", key)
	}
}

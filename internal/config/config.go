/*
Package config holds ototo's on-disk settings (spec 001 R2).

The settings live in one JSON file rather than in Fyne's preference store so
that the CLI and the GUI read the same document; the CLI has no Fyne app and
would otherwise need a second source of truth. The keys are the ones the
PyQt6 audio-source-switcher wrote, spelled the same, so a config.json from that
program is read as it is.
*/
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileEnv names the environment variable that overrides the config file path.
const FileEnv = "OTOTO_CONFIG"

// Mic link values with a fixed meaning. Any other value is a source name.
const (
	// MicAuto matches the input to the output by device properties.
	MicAuto = "auto"
	// MicDefault leaves the input alone when the output changes.
	MicDefault = "default"
)

// Config is the settings document.
type Config struct {
	// DevicePriority is the auto-switch order, highest first: "bt:<MAC>" for
	// a Bluetooth device, the sink name for anything else.
	DevicePriority []string `json:"device_priority"`
	// AutoSwitch turns the priority auto-switch on.
	AutoSwitch bool `json:"auto_switch"`
	// ArctisIdleMinutes is the headset's idle disconnect, 0 for never, else 1..90.
	ArctisIdleMinutes int `json:"arctis_idle_minutes"`
	// MicLinks maps a priority id to MicAuto, MicDefault or a source name.
	MicLinks map[string]string `json:"mic_links"`
	// OSDEnabled shows the program's own volume indicator.
	OSDEnabled bool `json:"osd_enabled"`
	// OSDTextSize is the indicator's value text size in points; 0 means
	// DefaultOSDTextSize.
	OSDTextSize int `json:"osd_text_size"`
	// OSDFont is the indicator's font family; "" means the window's.
	OSDFont string `json:"osd_font"`
	// SwitchNotifications sends a desktop notification on an automatic switch.
	// Failure notifications are sent regardless.
	SwitchNotifications bool `json:"switch_notifications"`
	// SwitchInOSD shows a switch in the volume indicator instead of a
	// desktop notification.
	SwitchInOSD bool `json:"switch_in_osd"`
	// SwitchSound plays a sound on the new output when the output switches:
	// the built-in chime, or the WAV file SwitchSoundFile names.
	// SwitchSoundDelay is how many seconds after a switch to a device that
	// just appeared the sound plays: headphones that just connected take
	// a while to render anything, a WH-1000XM6 a few seconds.
	SwitchSound      bool   `json:"switch_sound"`
	SwitchSoundFile  string `json:"switch_sound_file"`
	SwitchSoundDelay int    `json:"switch_sound_delay"`
	// Hotkeys are the keys of the person's own (spec 002): each runs one
	// action, with a device for a connect. HotkeysEnabled says whether they
	// are bound on this desktop; off, the list is kept and nothing is bound.
	Hotkeys        []Hotkey `json:"hotkeys"`
	HotkeysEnabled bool     `json:"hotkeys_enabled"`
	// LoopbackEnabled plays the line-in source through the current output.
	LoopbackEnabled bool `json:"loopback_enabled"`
	// LoopbackSource is the line-in source to play, for a machine with more
	// than one; "" means the first found.
	LoopbackSource string `json:"loopback_source"`
	// MoveStreams moves playing audio to the new output on a switch.
	MoveStreams bool `json:"move_streams"`
}

// DefaultOSDTextSize is the indicator's value text size when the setting
// is absent: large, because the indicator is read from across the room.
const DefaultOSDTextSize = 32

// Hotkey is one key of the person's own and what it runs.
type Hotkey struct {
	// Key as System Settings writes it: "Meta+A", "Ctrl+Alt+F5", "Meta+Num++".
	Key string `json:"key"`
	// Action is one of the HotkeyActions.
	Action string `json:"action"`
	// Device is the priority id the connect action switches to; "" for the
	// other actions.
	Device string `json:"device,omitempty"`
}

// The actions a hotkey can run.
const (
	HotkeyConnect = "connect"
	HotkeyVolUp   = "vol-up"
	HotkeyVolDown = "vol-down"
)

// HotkeyActions lists them.
var HotkeyActions = []string{HotkeyConnect, HotkeyVolUp, HotkeyVolDown}

// DefaultSwitchSoundDelay is the seconds a device that just appeared gets
// before the switch sound: a little after the slowest headphones seen, a
// WH-1000XM6, start to render.
const DefaultSwitchSoundDelay = 5

// Default is the document a machine starts with.
func Default() Config {
	return Config{
		DevicePriority:      []string{},
		MicLinks:            map[string]string{},
		OSDEnabled:          true,
		OSDTextSize:         DefaultOSDTextSize,
		SwitchNotifications: true,
		MoveStreams:         true,
		SwitchSoundDelay:    DefaultSwitchSoundDelay,
		HotkeysEnabled:      true,
	}
}

// DefaultPath is where the settings live when nothing says otherwise:
// $OTOTO_CONFIG, else $XDG_CONFIG_HOME/ototo/config.json.
func DefaultPath() (string, error) {
	if p := os.Getenv(FileEnv); p != "" {
		return ExpandPath(p), nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(ExpandPath(dir), "ototo", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "ototo", "config.json"), nil
}

// Resolve turns a --config value into the path to read: the value itself, or
// DefaultPath when it is empty.
func Resolve(path string) (string, error) {
	if path != "" {
		return ExpandPath(path), nil
	}
	return DefaultPath()
}

/*
Load reads the document at path, or the default one when path is empty.

A missing file is the default document and not an error: `ototo status` on a
machine that has never run ototo is a legitimate thing to run. Keys the file
does not carry keep their defaults, which is how a file written by the older
program, or by an older version of this one, is backfilled rather than refused.
*/
func Load(path string) (Config, string, error) {
	resolved, err := Resolve(path)
	if err != nil {
		return Config{}, "", err
	}
	cfg := Default()
	raw, err := os.ReadFile(resolved)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, resolved, nil
	}
	if err != nil {
		return Config{}, resolved, fmt.Errorf("read %s: %w", resolved, err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, resolved, fmt.Errorf("parse %s: %w", resolved, err)
	}
	if cfg.DevicePriority == nil {
		cfg.DevicePriority = []string{}
	}
	if cfg.MicLinks == nil {
		cfg.MicLinks = map[string]string{}
	}
	return cfg, resolved, nil
}

// Save writes the document atomically: to a temporary file beside the target,
// then renamed over it, so a crash mid-write leaves the old file intact.
func Save(path string, cfg Config) error {
	resolved, err := Resolve(path)
	if err != nil {
		return err
	}
	if err := MkdirAll(filepath.Dir(resolved)); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(resolved), ".config-*.json")
	if err != nil {
		return fmt.Errorf("write %s: %w", resolved, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", resolved, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", resolved, err)
	}
	if err := os.Rename(tmpName, resolved); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace %s: %w", resolved, err)
	}
	return nil
}

// ExpandPath resolves a leading "~". Only a leading one: a tilde in the middle
// of a path is an ordinary filename to every shell.
func ExpandPath(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// ErrTildeComponent reports a path with a component that is literally "~".
var ErrTildeComponent = errors.New("path component is a bare tilde")

/*
CheckCreatablePath refuses to create anything under a path component that is
literally "~" (angou's rule, kept in every sibling).

A directory named "~" is a trap: from inside its parent, the obvious way to
remove it is `rm -rf ~`, which the shell expands to the user's home directory
before rm ever runs.
*/
func CheckCreatablePath(p string) error {
	for _, part := range strings.Split(filepath.Clean(p), string(filepath.Separator)) {
		if part != "~" {
			continue
		}
		return fmt.Errorf("%w: %s\n"+
			"Refusing to create anything under a directory named \"~\". From its parent, the "+
			"obvious way to remove it is `rm -rf ~`, which the shell expands to your home "+
			"directory before rm runs.\n"+
			"If you meant your home directory, write it as ~/ at the start of the path, or "+
			"give the full path", ErrTildeComponent, p)
	}
	return nil
}

// MkdirAll creates a directory lazily, at the moment it is needed, and never
// under a bare-tilde component.
func MkdirAll(dir string) error {
	if err := CheckCreatablePath(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return nil
}

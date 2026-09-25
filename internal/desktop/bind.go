package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/godbus/dbus/v5"
)

/*
Hotkeys of the person's own (spec 001 R11.6): a key that switches to a
device, or steps the volume, registered the way the volume keys are. The
original offered "Copy Hotkey Command" and left the person to make the
shortcut in System Settings; here the program makes it, and a shortcut
that another program's entry held is released, since a key can run one
thing.

A binding is a command shortcut entry named for its key, so that unbinding
the key finds it without a record.
*/

// Qt modifier bits and the key codes this parser knows.
const (
	modShift  int32 = 0x02000000
	modCtrl   int32 = 0x04000000
	modAlt    int32 = 0x08000000
	modMeta   int32 = 0x10000000
	modKeypad int32 = 0x20000000
	keyF1     int32 = 0x01000030
)

var namedKeys = map[string]int32{
	"space": 0x20, "tab": 0x01000001, "return": 0x01000004, "enter": 0x01000005,
	"esc": 0x01000000, "escape": 0x01000000, "home": 0x01000010, "end": 0x01000011,
	"pgup": 0x01000016, "pgdown": 0x01000017, "insert": 0x01000006, "delete": 0x01000007,
	"left": 0x01000012, "up": 0x01000013, "right": 0x01000014, "down": 0x01000015,
	"volume up": keyVolumeUp, "volume down": keyVolumeDown, "volume mute": 0x01000071,
}

/*
ParseKey turns a shortcut as System Settings writes it, "Meta+A",
"Ctrl+Alt+F5", "Meta+Num++", into the Qt key combination kglobalaccel
takes. Modifiers are Meta, Ctrl, Alt and Shift; "Num+" marks the keypad;
a letter, a digit, a punctuation character, F1 to F35, or a named key ends
it.
*/
func ParseKey(text string) (int32, error) {
	mods, key, err := tokens(text)
	if err != nil {
		return 0, err
	}
	code, err := keyCode(key)
	if err != nil {
		return 0, err
	}
	return mods | code, nil
}

// tokens splits a shortcut into its modifier bits and its key. "Meta+Num++"
// splits into Meta, Num, "", "": two empty parts at the end mean the key is
// "+"; one means the text ended in a separator and names no key.
func tokens(text string) (int32, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, "", errors.New("no key given")
	}
	parts := strings.Split(text, "+")
	var mods int32
	var key string
	if n := len(parts); n >= 2 && parts[n-1] == "" && parts[n-2] == "" {
		key = "+"
		parts = parts[:n-2]
	}
	for _, p := range parts {
		switch strings.ToLower(p) {
		case "meta", "super", "win":
			mods |= modMeta
		case "ctrl", "control":
			mods |= modCtrl
		case "alt":
			mods |= modAlt
		case "shift":
			mods |= modShift
		case "num":
			mods |= modKeypad
		case "":
			return 0, "", fmt.Errorf("%q names no key", text)
		default:
			if key != "" {
				return 0, "", fmt.Errorf("%q has two keys", text)
			}
			key = p
		}
	}
	if key == "" {
		return 0, "", fmt.Errorf("%q names no key", text)
	}
	return mods, key, nil
}

var fKey = regexp.MustCompile(`^[Ff]([1-9]|[12][0-9]|3[0-5])$`)

func keyCode(key string) (int32, error) {
	if m := fKey.FindStringSubmatch(key); m != nil {
		n, _ := strconv.Atoi(m[1])
		return keyF1 + int32(n-1), nil //nolint:gosec // 1..35
	}
	if code, ok := namedKeys[strings.ToLower(key)]; ok {
		return code, nil
	}
	runes := []rune(key)
	if len(runes) == 1 && runes[0] < 0x80 && !unicode.IsSpace(runes[0]) {
		return unicode.ToUpper(runes[0]), nil
	}
	return 0, fmt.Errorf("key %q is not one this program knows", key)
}

// slug is the file-safe form of a shortcut: "Meta+Num++" is
// "meta-num-plus", "Meta+Num+-" is "meta-num-minus".
func slug(text string) string {
	mods, key, err := tokens(text)
	if err != nil {
		return strings.ToLower(strings.NewReplacer("+", "-", " ", "-").Replace(strings.TrimSpace(text)))
	}
	var out []string
	for _, m := range []struct {
		bit  int32
		name string
	}{{modMeta, "meta"}, {modCtrl, "ctrl"}, {modAlt, "alt"}, {modShift, "shift"}, {modKeypad, "num"}} {
		if mods&m.bit != 0 {
			out = append(out, m.name)
		}
	}
	switch key {
	case "+":
		out = append(out, "plus")
	case "-":
		out = append(out, "minus")
	default:
		out = append(out, strings.ToLower(strings.ReplaceAll(key, " ", "-")))
	}
	return strings.Join(out, "-")
}

// bindEntry is the desktop entry name for a key.
func bindEntry(key string) string { return "io.ushineko.ototo.bind-" + slug(key) + ".desktop" }

/*
Bind makes key run ototo with args, such as {"--connect", "AirPods Pro"} or
{"--vol-up"}. A command shortcut entry that held the key is unregistered
first and named in released; another program's own shortcut is left where
it is and the registration is refused, since a program's own keys are its
to give up in System Settings.
*/
func Bind(ctx context.Context, key string, args []string) (released []string, err error) {
	code, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("nothing to run on the key")
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to the session bus: %w", err)
	}
	apps, err := applicationsDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(apps, 0o750); err != nil {
		return nil, fmt.Errorf("create %s: %w", apps, err)
	}
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, quoteExec(a))
	}
	entry := bindEntry(key)
	name := "ototo: " + strings.Join(args, " ")
	if err := os.WriteFile(filepath.Join(apps, entry), []byte(commandEntry(name, "ototo "+strings.Join(quoted, " "))), 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", entry, err)
	}

	// A command shortcut holding the key is released; the shortcuts file
	// names it by the key as the desktop writes it.
	shortcuts, err := ShortcutsPath()
	if err != nil {
		return nil, err
	}
	text, _ := os.ReadFile(shortcuts) //nolint:gosec // the user's own config
	for _, h := range serviceHolders(string(text), key) {
		if h == entry {
			continue
		}
		var ok bool
		if err := accel(conn).CallWithContext(ctx, accelIface+".unregister", 0, h, launch).Store(&ok); err == nil && ok {
			released = append(released, h)
		}
	}
	return released, register(ctx, conn, actionID(entry, launch, name), code)
}

// Unbind removes ototo's shortcut for key, and says whether there was one.
func Unbind(ctx context.Context, key string) (bool, error) {
	if _, err := ParseKey(key); err != nil {
		return false, err
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return false, fmt.Errorf("connect to the session bus: %w", err)
	}
	entry := bindEntry(key)
	var ok bool
	if err := accel(conn).CallWithContext(ctx, accelIface+".unregister", 0, entry, launch).Store(&ok); err != nil {
		return false, fmt.Errorf("unregister %s: %w", entry, err)
	}
	apps, err := applicationsDir()
	if err != nil {
		return ok, err
	}
	if err := os.Remove(filepath.Join(apps, entry)); err != nil && !os.IsNotExist(err) {
		return ok, fmt.Errorf("remove %s: %w", entry, err)
	}
	return ok, nil
}

// serviceHolders are the command shortcut entries whose key is key, as the
// shortcuts file spells it.
func serviceHolders(text, key string) []string {
	want := strings.TrimSpace(key)
	var out []string
	section := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "[services]["); ok {
			section = strings.TrimSuffix(rest, "]")
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = ""
			continue
		}
		if section == "" {
			continue
		}
		if value, ok := strings.CutPrefix(line, launch+"="); ok {
			for _, seq := range strings.Split(value, "\t") {
				if strings.EqualFold(strings.TrimSpace(seq), want) {
					out = append(out, section)
				}
			}
		}
	}
	return out
}

// quoteExec quotes one Exec argument the way the desktop entry
// specification reads it: double quotes, with the characters it reserves
// escaped.
func quoteExec(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\"'\\$`%") {
		return a
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`, "%", "%%")
	return `"` + r.Replace(a) + `"`
}

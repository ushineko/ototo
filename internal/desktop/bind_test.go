package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseKeyReadsWhatSystemSettingsWrites: the spellings in
// kglobalshortcutsrc, including the keypad plus, whose text ends in "+".
func TestParseKeyReadsWhatSystemSettingsWrites(t *testing.T) {
	cases := map[string]int32{
		"Meta+A":            modMeta | 'A',
		"meta+z":            modMeta | 'Z',
		"Ctrl+Alt+F5":       modCtrl | modAlt | keyF1 + 4,
		"Meta+Num++":        modMeta | modKeypad | '+',
		"Meta+Num+-":        modMeta | modKeypad | '-',
		"Volume Up":         keyVolumeUp,
		"Shift+Volume Down": modShift | keyVolumeDown,
		"F12":               keyF1 + 11,
	}
	for text, want := range cases {
		got, err := ParseKey(text)
		require.NoError(t, err, text)
		require.Equalf(t, want, got, "%s", text)
	}
	for _, bad := range []string{"", "Meta+", "Meta+A+B", "Meta+Hyper"} {
		_, err := ParseKey(bad)
		require.Error(t, err, bad)
	}
}

// TestABindingIsNamedForItsKey: unbinding needs no record.
func TestABindingIsNamedForItsKey(t *testing.T) {
	require.Equal(t, "io.ushineko.ototo.bind-meta-a.desktop", bindEntry("Meta+A"))
	require.Equal(t, "io.ushineko.ototo.bind-meta-num-plus.desktop", bindEntry("Meta+Num++"))
	require.Equal(t, "io.ushineko.ototo.bind-meta-num-minus.desktop", bindEntry("Meta+Num+-"))
	require.Equal(t, "io.ushineko.ototo.bind-volume-up.desktop", bindEntry("Volume Up"))
}

// TestExecArgumentsAreQuotedForTheDesktopEntry: a device name with a space
// is one argument, and the characters the entry format reserves are escaped.
func TestExecArgumentsAreQuotedForTheDesktopEntry(t *testing.T) {
	require.Equal(t, "--vol-up", quoteExec("--vol-up"))
	require.Equal(t, `"AirPods Pro"`, quoteExec("AirPods Pro"))
	require.Equal(t, `"Papa\"s 100%%"`, quoteExec(`Papa"s 100%`))
}

// TestBindingsAreListedFromTheEntriesAndTheShortcutsFile: an entry's key
// comes from the shortcuts file when it has one, else from the entry's
// name; the Exec is read back into arguments, quotes undone.
func TestBindingsAreListedFromTheEntriesAndTheShortcutsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	apps := filepath.Join(home, "data", "applications")
	require.NoError(t, os.MkdirAll(apps, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "config"), 0o750))
	write := func(name, exec string) {
		require.NoError(t, os.WriteFile(filepath.Join(apps, name), []byte(commandEntry("ototo: x", exec)), 0o600))
	}
	write(bindEntry("Meta+A"), `ototo --connect "AirPods Pro"`)
	write(bindEntry("Meta+Num++"), "ototo --vol-up")
	write(bindEntry("Meta+Z"), "ototo --something-else")
	write("io.ushineko.ototo.vol-up.desktop", "ototo --vol-up") // not a binding
	require.NoError(t, os.WriteFile(filepath.Join(home, "config", "kglobalshortcutsrc"),
		[]byte("[services]["+bindEntry("Meta+A")+"]\n_launch=Meta+A\n\n[services]["+bindEntry("Meta+Num++")+"]\n_launch=none\n"), 0o600))

	got, err := ListBindings()
	require.NoError(t, err)
	byEntry := map[string]Binding{}
	for _, b := range got {
		byEntry[b.Entry] = b
	}
	require.Len(t, byEntry, 3)
	a := byEntry[bindEntry("Meta+A")]
	require.Equal(t, Binding{Entry: bindEntry("Meta+A"), Key: "Meta+A", Args: []string{"--connect", "AirPods Pro"}, Bound: true}, a)
	plus := byEntry[bindEntry("Meta+Num++")]
	require.Equal(t, "Meta+Num++", plus.Key, "the key was not read back from the entry's name")
	require.False(t, plus.Bound)
	require.Equal(t, []string{"--vol-up"}, plus.Args)
	require.Equal(t, "Meta+Z", byEntry[bindEntry("Meta+Z")].Key)
}

// TestExecIsReadBackAsItWasWritten: unquoteExec undoes quoteExec.
func TestExecIsReadBackAsItWasWritten(t *testing.T) {
	for _, args := range [][]string{
		{"--connect", "AirPods Pro"},
		{"--connect", `a "quoted" $name 100%`},
		{"--vol-up"},
		{"--connect", "bt:AA:BB:CC:DD:EE:FF"},
	} {
		quoted := make([]string, 0, len(args))
		for _, a := range args {
			quoted = append(quoted, quoteExec(a))
		}
		require.Equal(t, append([]string{"ototo"}, args...), unquoteExec("ototo "+strings.Join(quoted, " ")), args)
	}
}

// TestASlugReadsBack: the entry name's slug names the key again.
func TestASlugReadsBack(t *testing.T) {
	for _, key := range []string{"Meta+A", "Meta+Num++", "Meta+Num+-", "Ctrl+Alt+F5", "Shift+Space"} {
		require.Equal(t, key, unslug(slug(key)), key)
	}
}

// TestTheBindRecordRoundTrips: what a binding released is written per
// entry and read back, and an empty record is no file.
func TestTheBindRecordRoundTrips(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	rec, err := readBindRecord()
	require.NoError(t, err)
	require.Empty(t, rec)
	rec[bindEntry("Meta+A")] = []Holder{{Component: "old.desktop", Action: launch, Key: 0x10000041, Service: true}}
	require.NoError(t, writeBindRecord(rec))
	got, err := readBindRecord()
	require.NoError(t, err)
	require.Equal(t, rec, got)
	require.NoError(t, writeBindRecord(map[string][]Holder{}))
	p, _ := bindRecordPath()
	require.NoFileExists(t, p)
}

// TestUnsupportedWithoutTheBus: no session bus, no bindings.
func TestUnsupportedWithoutTheBus(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")
	require.False(t, Supported(context.Background()))
}

package desktop

import (
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

// TestTheOldHolderOfAKeyIsFound: the entry that held Meta+A is what Bind
// releases, and ototo's own entry for the key is not.
func TestTheOldHolderOfAKeyIsFound(t *testing.T) {
	text := "[services][net.local.python3.desktop]\n_launch=Meta+A\n\n[services][other.desktop]\n_launch=Meta+Z\n\n[kmix]\nmute=Meta+A,none,Mute\n"
	require.Equal(t, []string{"net.local.python3.desktop"}, serviceHolders(text, "Meta+A"))
	require.Empty(t, serviceHolders(text, "Meta+X"))
}

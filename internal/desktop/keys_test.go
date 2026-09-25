package desktop

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const shortcuts = `[$Version]
update_info=kglobalshortcutsrc.upd:x

[kmix]
_k_friendly_name=Audio Volume
decrease_volume=none,Volume Down,Decrease Volume
increase_volume=Volume Up	Meta+F12,Volume Up,Increase Volume
mute=Volume Mute,Volume Mute,Mute

[services][net.local.old-switcher-4.desktop]
_launch=Volume Down

[services][io.ushineko.ototo.vol-up.desktop]
_launch=Volume Up

[services][net.local.other.desktop]
_launch=Meta+Num++
`

// TestHoldersAreWhatHoldsTheKeysNow: the mixer's current sequence counts
// and its default does not; a command shortcut counts; ototo's own does
// not, or a second install would record itself.
func TestHoldersAreWhatHoldsTheKeysNow(t *testing.T) {
	got := holders(shortcuts)
	require.Equal(t, []Holder{
		{Component: "kmix", Action: "increase_volume", Key: keyVolumeUp},
		{Component: "net.local.old-switcher-4.desktop", Action: "_launch", Key: keyVolumeDown, Service: true},
	}, got)
}

// TestTheCommandEntriesRunOtoto: what KDE launches on the key.
func TestTheCommandEntriesRunOtoto(t *testing.T) {
	e := commandEntry("ototo: volume up", "ototo --vol-up")
	require.Contains(t, e, "Exec=ototo --vol-up\n")
	require.Contains(t, e, "X-KDE-GlobalAccel-CommandShortcut=true\n")
	require.Contains(t, e, "NoDisplay=true\n")
	require.Equal(t, "Volume Up", keyName(keyVolumeUp))
}

package bluetooth

import (
	"context"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"
)

func device(mac, alias, name, icon string, uuids []string, connected bool) map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{
		deviceIface: {
			"Address":   dbus.MakeVariant(mac),
			"Alias":     dbus.MakeVariant(alias),
			"Name":      dbus.MakeVariant(name),
			"Icon":      dbus.MakeVariant(icon),
			"UUIDs":     dbus.MakeVariant(uuids),
			"Connected": dbus.MakeVariant(connected),
		},
	}
}

// TestOnlyAudioDevicesAreListed: a keyboard is paired too, and it is not an
// output. A device with no profile list but an audio icon still counts.
func TestOnlyAudioDevicesAreListed(t *testing.T) {
	objs := managed{
		"/org/bluez/hci0": {"org.bluez.Adapter1": {}},
		"/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF": device("AA:BB:CC:DD:EE:FF", "AirPods Pro", "AirPods", "audio-headset",
			[]string{"0000110B-0000-1000-8000-00805F9B34FB"}, true),
		"/org/bluez/hci0/dev_00_11_22_33_44_55": device("00:11:22:33:44:55", "", "Keyboard", "input-keyboard",
			[]string{"00001124-0000-1000-8000-00805f9b34fb"}, true),
		"/org/bluez/hci1/dev_00_11_22_33_44_66": device("00:11:22:33:44:66", "", "", "audio-card", nil, false),
	}
	got := filterAudio(objs)
	require.Len(t, got, 2)
	require.Equal(t, "00:11:22:33:44:66", got[0].MAC, "a device with no name is named by its address")
	require.Equal(t, "00:11:22:33:44:66", got[0].Name)
	require.Equal(t, "AirPods Pro", got[1].Name, "the alias outranks the name")
	require.True(t, got[1].Connected)

	path, ok := pathFor(objs, "aa:bb:cc:dd:ee:ff")
	require.True(t, ok, "the address match is case-insensitive")
	require.Equal(t, dbus.ObjectPath("/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF"), path)
	_, ok = pathFor(objs, "FF:FF:FF:FF:FF:FF")
	require.False(t, ok)
}

// TestTheLiveAdapterAnswers reads the paired devices where BlueZ runs, and
// only reads. The list is whatever the machine has.
func TestTheLiveAdapterAnswers(t *testing.T) {
	a, err := System()
	if err != nil {
		t.Skipf("no system bus: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	list, err := a.Devices(ctx)
	if err != nil {
		t.Skipf("BlueZ did not answer: %v", err)
	}
	for _, d := range list {
		require.Len(t, d.MAC, 17)
		require.NotEmpty(t, d.Name)
	}
}

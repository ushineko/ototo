package devices

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
)

// The addresses below are the documentation range, not a device of anyone's.
const (
	airpodsMAC  = "AA:BB:CC:DD:EE:FF"
	airpodsSink = "bluez_output.AA_BB_CC_DD_EE_FF.1"
)

// TestAPriorityIDSurvivesASinkRename: a Bluetooth sink is keyed by its
// address, because the server renames the sink between profiles and a
// config.json keyed by sink name would forget the device every time.
func TestAPriorityIDSurvivesASinkRename(t *testing.T) {
	require.Equal(t, "bt:"+airpodsMAC, PriorityID(airpodsSink))
	require.Equal(t, "bt:"+airpodsMAC, PriorityID("bluez_output.aa_bb_cc_dd_ee_ff.a2dp-sink"))
	require.Equal(t, "bt:"+airpodsMAC, PriorityID("bluez_sink.AA:BB:CC:DD:EE:FF.headset_head_unit"))
	require.Equal(t, "alsa_output.usb-Generic_DAC-00.analog-stereo", PriorityID("alsa_output.usb-Generic_DAC-00.analog-stereo"))
}

func sink(name string, props map[string]string) audio.Device {
	return audio.Device{Name: name, Properties: props}
}

// TestDisplayNameFallsThroughInTheOriginalsOrder: alias, vendor+product,
// description, sink name, with "(null)" scrubbed.
func TestDisplayNameFallsThroughInTheOriginalsOrder(t *testing.T) {
	aliases := map[string]string{airpodsMAC: "AirPods Pro"}

	require.Equal(t, "AirPods Pro",
		DisplayName(sink(airpodsSink, map[string]string{"device.api": "bluez5", "bluez.alias": "stale"}), aliases))
	require.Equal(t, "From props",
		DisplayName(sink(airpodsSink, map[string]string{"bluez.alias": "From props"}), nil))
	require.Equal(t, "Yeti Nano",
		DisplayName(sink("alsa_output.usb", map[string]string{"device.vendor.name": "Yeti", "device.product.name": "Nano"}), nil))
	require.Equal(t, "Acme DAC",
		DisplayName(sink("alsa_output.usb", map[string]string{"device.vendor.name": "Acme", "device.model": "DAC"}), nil))
	require.Equal(t, "Built-in Audio",
		DisplayName(sink("alsa_output.pci", map[string]string{"device.vendor.name": "Only vendor", "device.description": "Built-in Audio"}), nil))
	require.Equal(t, "Speakers",
		DisplayName(sink("alsa_output.pci", map[string]string{"device.description": "(null) Speakers (null)"}), nil))
	require.Equal(t, "alsa_output.raw",
		DisplayName(sink("alsa_output.raw", map[string]string{"device.description": "(null)"}), nil))
}

// TestAnUnpluggedJackIsDisconnectedAndSaysSo: the auto-switch skips a sink
// whose active port has nothing in it, and the row tells the reader why.
func TestAnUnpluggedJackIsDisconnectedAndSaysSo(t *testing.T) {
	s := sink("alsa_output.pci.analog-stereo", map[string]string{"device.description": "Built-in Audio"})
	s.ActivePort = "analog-output-headphones"
	s.Ports = []audio.Port{
		{Name: "analog-output-lineout", Description: "Line Out", Availability: audio.AvailabilityYes},
		{Name: "analog-output-headphones", Description: "Headphones", Availability: audio.AvailabilityNo},
	}
	rows := List(Inputs{Sinks: []audio.Device{s}})
	require.Len(t, rows, 1)
	require.Equal(t, "Built-in Audio - Headphones [Disconnected]", rows[0].Name)
	require.True(t, rows[0].Online)
	require.False(t, rows[0].Connected)

	s.ActivePort = "analog-output-lineout"
	rows = List(Inputs{Sinks: []audio.Device{s}})
	require.Equal(t, "Built-in Audio - Line Out", rows[0].Name)
	require.True(t, rows[0].Connected)
}

// TestAnalogOutputIsNotWorthMentioning: the one port description the
// original left off, because every plain sink has it.
func TestAnalogOutputIsNotWorthMentioning(t *testing.T) {
	s := sink("alsa_output.usb", map[string]string{"device.description": "USB DAC"})
	s.ActivePort = "analog-output"
	s.Ports = []audio.Port{{Name: "analog-output", Description: "Analog Output", Availability: audio.AvailabilityUnknown}}
	rows := List(Inputs{Sinks: []audio.Device{s}})
	require.Equal(t, "USB DAC", rows[0].Name)
}

// TestTheHeadsetShowsItsBatteryOrIsOff: headsetcontrol answering nothing
// means the headset is powered off, so the auto-switch must move on.
func TestTheHeadsetShowsItsBatteryOrIsOff(t *testing.T) {
	s := sink("alsa_output.usb-SteelSeries_Arctis_Nova_Pro_Wireless-00.analog-stereo",
		map[string]string{"device.vendor.name": "SteelSeries", "device.product.name": "Arctis Nova Pro Wireless"})
	rows := List(Inputs{Sinks: []audio.Device{s}, Headset: Headset{Detected: true, Battery: "87%"}})
	require.Equal(t, "SteelSeries Arctis Nova Pro Wireless [87%]", rows[0].Name)
	require.True(t, rows[0].Connected)

	rows = List(Inputs{Sinks: []audio.Device{s}})
	require.Equal(t, "SteelSeries Arctis Nova Pro Wireless [Disconnected]", rows[0].Name)
	require.False(t, rows[0].Connected)
}

// TestTheListIsPriorityThenAppearanceThenPaired: the order a person set
// comes first, remembered devices hold their place while away, the rest of
// the sinks follow in the server's order, and a paired headset never chosen
// is still offered.
func TestTheListIsPriorityThenAppearanceThenPaired(t *testing.T) {
	speakers := sink("alsa_output.pci", map[string]string{"device.description": "Speakers"})
	dac := sink("alsa_output.usb", map[string]string{"device.description": "USB DAC"})
	jdsp := sink(JamesDSPSink, map[string]string{"device.description": "JamesDSP Sink"})
	in := Inputs{
		Sinks:       []audio.Device{jdsp, speakers, dac},
		DefaultSink: "alsa_output.usb",
		Priority:    []string{"bt:" + airpodsMAC, JamesDSPSink, "alsa_output.usb", "bt:" + airpodsMAC},
		Bluetooth: []Bluetooth{
			{MAC: airpodsMAC, Name: "AirPods Pro"},
			{MAC: "00:11:22:33:44:55", Name: "Car stereo"},
		},
	}
	rows := List(in)
	ids := make([]string, len(rows))
	names := make([]string, len(rows))
	for i, r := range rows {
		ids[i], names[i] = r.ID, r.Name
	}
	require.Equal(t, []string{"bt:" + airpodsMAC, "alsa_output.usb", "alsa_output.pci", "bt:00:11:22:33:44:55"}, ids)
	require.Equal(t, []string{"AirPods Pro [Disconnected]", "USB DAC", "Speakers", "Car stereo [Disconnected]"}, names)
	require.True(t, rows[1].Default)
	require.False(t, rows[0].Online)
	require.Equal(t, airpodsMAC, rows[0].MAC)
	for _, r := range rows {
		require.NotEqual(t, JamesDSPSink, r.Sink, "the JamesDSP sink reached the list")
	}
}

// TestARememberedDeviceWithNoAdapterKeepsItsId: with no Bluetooth cache the
// row is named by its id rather than dropped, so the priority order stays
// editable on a machine whose adapter is off.
func TestARememberedDeviceWithNoAdapterKeepsItsId(t *testing.T) {
	rows := List(Inputs{Priority: []string{"bt:" + airpodsMAC, "alsa_output.gone"}})
	require.Len(t, rows, 2)
	require.Equal(t, "bt:"+airpodsMAC+" [Disconnected]", rows[0].Name)
	require.Equal(t, "alsa_output.gone [Disconnected]", rows[1].Name)
}

// TestAConnectedBluetoothSinkIsOneRow: the live sink and the paired entry
// are the same device and must not appear twice.
func TestAConnectedBluetoothSinkIsOneRow(t *testing.T) {
	s := sink(airpodsSink, map[string]string{"device.api": "bluez5"})
	rows := List(Inputs{
		Sinks:     []audio.Device{s},
		Bluetooth: []Bluetooth{{MAC: airpodsMAC, Name: "AirPods Pro", Connected: true}},
	})
	require.Len(t, rows, 1)
	require.Equal(t, "AirPods Pro", rows[0].Name)
	require.True(t, rows[0].Online)
}

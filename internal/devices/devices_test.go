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

// TestACurlyApostropheIsMadePlain: a phone names a headset "Papa’s"; the
// window's font has no such glyph, and the row it was in drew blank.
func TestACurlyApostropheIsMadePlain(t *testing.T) {
	rows := List(Inputs{Bluetooth: []Bluetooth{{MAC: airpodsMAC, Name: "Papa’s AirPods Pro"}}})
	require.Equal(t, "Papa's AirPods Pro [Disconnected]", rows[0].Name)
	require.Equal(t, `"Buds" - 1`, Plain("“Buds” – 1"))
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

// usbProps is a USB sink's naming properties: the database's vendor and
// product, and the card's own name and long name. The bus path is invented.
func usbProps(vendor, product, card, long string) map[string]string {
	return map[string]string{
		"device.bus": "usb", "device.vendor.name": vendor, "device.product.name": product,
		"alsa.card_name": card, "alsa.long_card_name": long + " at usb-0000:00:14.0-1, full speed",
	}
}

// TestAUSBDeviceIsNamedByItsOwnStrings (spec 004 R1): the usb.ids database
// names a device by the chip's ids, so a transmitter built on a CSR chip
// read as an Mpow headset, and a DAC on a Realtek chip carried Realtek's
// name. The device's own card name is believed where the two disagree, and
// a vendor the device does not claim is left off. A PCI device keeps the
// database name, which is the better one there.
func TestAUSBDeviceIsNamedByItsOwnStrings(t *testing.T) {
	cases := []struct {
		name  string
		props map[string]string
		want  string
	}{
		{"chip ids name a different product", usbProps("Cambridge Silicon Radio, Ltd",
			"Mpow HC5 Headset in charging mode - HID / Mass Storage", "UGREEN-BT701", "TaiYiLian UGREEN-BT701"), "UGREEN-BT701"},
		{"right product, chip maker as vendor", usbProps("Realtek Semiconductor Corp.",
			"CUBILUX CB5", "CUBILUX CB5", "Generic CUBILUX CB5"), "CUBILUX CB5"},
		{"the device agrees with the database", usbProps("SteelSeries ApS",
			"Arctis Nova Pro Wireless", "Arctis Nova Pro Wireless", "SteelSeries Arctis Nova Pro Wireless"),
			"SteelSeries ApS Arctis Nova Pro Wireless"},
		{"a card name ALSA cut short agrees", usbProps("Acme",
			"Studio Monitor Interface Pro Edition Mk II", "Studio Monitor Interface Pro Edi", "Acme Studio Monitor Interface Pro Edi"),
			"Acme Studio Monitor Interface Pro Edition Mk II"},
		{"no long name keeps the vendor", map[string]string{"device.bus": "usb", "device.vendor.name": "Acme",
			"device.product.name": "Wrong Thing", "alsa.card_name": "Desk DAC"}, "Acme Desk DAC"},
		{"no card name is R4.2", map[string]string{"device.bus": "usb", "device.vendor.name": "Acme",
			"device.product.name": "DAC"}, "Acme DAC"},
		{"PCI keeps the database name", map[string]string{"device.bus": "pci", "device.vendor.name": "NVIDIA Corporation",
			"device.product.name": "AD102 High Definition Audio Controller", "alsa.card_name": "HDA NVidia",
			"alsa.long_card_name": "HDA NVidia at 0xf0000000 irq 17"}, "NVIDIA Corporation AD102 High Definition Audio Controller"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, DisplayName(sink("alsa_output.x", c.props), nil))
		})
	}

	arctis := sink("alsa_output.usb-SteelSeries_Arctis_Nova_Pro_Wireless-00.analog-stereo", cases[2].props)
	rows := List(Inputs{Sinks: []audio.Device{arctis}, Headset: Headset{Detected: true, Battery: "87%"}})
	require.Equal(t, "SteelSeries ApS Arctis Nova Pro Wireless [87%]", rows[0].Name, "the Arctis is no longer the headset")
}

// TestAnAwayDeviceKeepsTheNameItHadWhenSeen (spec 004 R2): an HDMI output
// whose sink is gone read as its sink name; it reads as what it was called,
// port and all, and only a device never seen falls back to the id.
func TestAnAwayDeviceKeepsTheNameItHadWhenSeen(t *testing.T) {
	hdmi := sink("alsa_output.pci-0000_01_00.1.hdmi-stereo-extra1", map[string]string{"device.description": "HDMI Audio"})
	hdmi.ActivePort = "hdmi-output-1"
	hdmi.Ports = []audio.Port{{Name: "hdmi-output-1", Description: "HDMI / DisplayPort 2", Availability: audio.AvailabilityNo}}
	live := List(Inputs{Sinks: []audio.Device{hdmi}})
	require.Equal(t, "HDMI Audio - HDMI / DisplayPort 2 [Disconnected]", live[0].Name)
	require.Equal(t, "HDMI Audio - HDMI / DisplayPort 2", live[0].Label, "the remembered name carries a state")

	rows := List(Inputs{
		Priority: []string{hdmi.Name, "alsa_output.never-seen", "bt:" + airpodsMAC},
		Names:    map[string]string{hdmi.Name: live[0].Label, "bt:" + airpodsMAC: "a stale name"},
	})
	require.Equal(t, "HDMI Audio - HDMI / DisplayPort 2 [Disconnected]", rows[0].Name)
	require.False(t, rows[0].Online)
	require.Equal(t, "alsa_output.never-seen [Disconnected]", rows[1].Name)
	require.Equal(t, "bt:"+airpodsMAC+" [Disconnected]", rows[2].Name, "a Bluetooth id is named by the adapter only")
}

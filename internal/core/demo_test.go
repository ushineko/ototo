package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
)

const demoJSON = `{
  "default_sink": "alsa_output.usb-Example_DAC-00.analog-stereo",
  "default_source": "alsa_input.usb-Example_DAC-00.mono",
  "sinks": [
    {"name": "alsa_output.usb-Example_DAC-00.analog-stereo", "description": "Example DAC",
     "properties": {"device.vendor.name": "Example", "device.product.name": "DAC"}, "volume": 40},
    {"name": "bluez_output.AA_BB_CC_DD_EE_FF.1", "description": "Buds",
     "properties": {"device.api": "bluez5", "api.bluez5.address": "AA:BB:CC:DD:EE:FF"}, "volume": 70}
  ],
  "sources": [
    {"name": "alsa_input.usb-Example_DAC-00.mono", "description": "Desk mic", "properties": {"device.description": "Desk mic"}},
    {"name": "bluez_input.AA_BB_CC_DD_EE_FF.0", "properties": {"api.bluez5.address": "AA:BB:CC:DD:EE:FF", "device.description": "Buds mic"}}
  ],
  "bluetooth": [{"mac": "AA:BB:CC:DD:EE:FF", "name": "Example Buds", "connected": true},
                {"mac": "00:11:22:33:44:55", "name": "Car stereo"}],
  "headset": {"detected": true, "battery": "87%"}
}`

// TestTheDemoServerRunsTheWholePath: status, a switch and a volume step,
// over a file and nothing of the machine's, which is what a screenshot needs.
func TestTheDemoServerRunsTheWholePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path := filepath.Join(home, "devices.json")
	require.NoError(t, os.WriteFile(path, []byte(demoJSON), 0o600))
	req := Request{Server: audio.DemoPrefix + path}

	res, err := Status(context.Background(), StatusRequest{Request: req})
	require.NoError(t, err)
	require.Empty(t, res.ServerError)
	require.Equal(t, "demo server", res.Server.Name)
	var names []string
	for _, d := range res.Devices {
		names = append(names, d.Name)
	}
	require.Equal(t, []string{"Example DAC", "Example Buds", "Car stereo [Disconnected]"}, names)
	require.True(t, res.Devices[0].Default)

	sw := NewSwitcher()
	out, err := sw.Switch(context.Background(), SwitchRequest{Request: req, Target: "buds"})
	require.NoError(t, err)
	require.Equal(t, "Example Buds", out.Device.Name)
	require.Equal(t, "bluez_input.AA_BB_CC_DD_EE_FF.0", out.Mic)

	vol, err := sw.Volume(context.Background(), VolumeRequest{Request: req, Delta: audio.VolumeStep})
	require.NoError(t, err)
	require.Equal(t, 75, vol.Percent)
	require.Equal(t, "bluez_output.AA_BB_CC_DD_EE_FF.1", vol.Sink)
}

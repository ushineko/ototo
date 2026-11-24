package audio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfreymuth/pulse/proto"
	"github.com/stretchr/testify/require"
)

// TestPercentRoundsLikePactl: 65536 is 100%, and the loudest channel decides,
// which is what `pactl get-sink-volume` reports and what the OSD showed.
func TestPercentRoundsLikePactl(t *testing.T) {
	require.Equal(t, 100, Percent(proto.ChannelVolumes{0x10000, 0x10000}))
	require.Equal(t, 0, Percent(proto.ChannelVolumes{0, 0}))
	require.Equal(t, 50, Percent(proto.ChannelVolumes{0x8000, 0x7fff}))
	require.Equal(t, 150, Percent(proto.ChannelVolumes{0x18000}))
	require.Equal(t, 0, Percent(nil))
}

// TestOnlyAPortThatSaysNoIsDisconnected: a USB DAC has no ports at all and
// must still count as connected, or the auto-switch would never pick it.
func TestOnlyAPortThatSaysNoIsDisconnected(t *testing.T) {
	require.True(t, Device{}.Connected())
	require.True(t, Device{ActivePort: "x", Ports: []Port{{Name: "x", Availability: AvailabilityUnknown}}}.Connected())
	require.True(t, Device{ActivePort: "x", Ports: []Port{{Name: "x", Availability: AvailabilityYes}}}.Connected())
	require.False(t, Device{ActivePort: "x", Ports: []Port{{Name: "x", Availability: AvailabilityNo}}}.Connected())
}

// TestTheLiveServerAnswers runs only where a sound server is listening. It is
// the one check that the protocol client works against PipeWire's pulse
// socket, which no fake can stand in for.
func TestTheLiveServerAnswers(t *testing.T) {
	sock := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "pulse", "native")
	if os.Getenv("PULSE_SERVER") == "" {
		if _, err := os.Stat(sock); err != nil {
			t.Skipf("no sound server socket at %s", sock)
		}
	}
	c, err := Connect("")
	if err != nil {
		t.Skipf("no sound server answered: %v", err)
	}
	defer func() { _ = c.Close() }()

	srv, err := c.Server()
	require.NoError(t, err)
	require.NotEmpty(t, srv.Name)

	sinks, err := c.Sinks()
	require.NoError(t, err)
	for _, s := range sinks {
		require.NotEmpty(t, s.Name)
	}
	_, err = c.Sources()
	require.NoError(t, err)
}

// TestTheDemoSinkStateDefaultsToRunning (spec 007 R1.2): a demo file
// written before sinks had a state keeps playing as before, and a file can
// put a sink to sleep to show the wake.
func TestTheDemoSinkStateDefaultsToRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"sinks": [
		{"name": "alsa_output.plain"},
		{"name": "bluez_output.AA_BB_CC_DD_EE_FF.1", "state": "suspended"},
		{"name": "alsa_output.idle", "state": "idle"}]}`), 0o600))
	d, err := LoadDemo(DemoPrefix + path)
	require.NoError(t, err)
	sinks, err := d.Sinks()
	require.NoError(t, err)
	require.Equal(t, StateRunning, sinks[0].State)
	require.Equal(t, StateSuspended, sinks[1].State)
	require.True(t, sinks[1].State.Asleep())
	require.True(t, sinks[2].State.Asleep())
	require.False(t, StateUnknown.Asleep(), "a sink of unknown state was taken for asleep")
}

// TestTheSinkStateIsThePulseValue (spec 007 R1.1): pa_sink_state_t, as
// PipeWire's pulse server sends it; anything else is unknown.
func TestTheSinkStateIsThePulseValue(t *testing.T) {
	require.Equal(t, StateRunning, sinkState(0))
	require.Equal(t, StateIdle, sinkState(1))
	require.Equal(t, StateSuspended, sinkState(2))
	require.Equal(t, StateUnknown, sinkState(0xffffffff))
}

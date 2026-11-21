package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Shapes captured from pw-link on a machine with the bridge (spec 006), with
// an invented headset address.
const (
	headset = "bluez_output.00_11_22_33_44_55.1"

	bridgeOutputs = `jdsp_@PwJamesDspPlugin_JamesDsp:output_FL
jdsp_@PwJamesDspPlugin_JamesDsp:output_FR
ototo_bt_bridge_out:output_FR
ototo_bt_bridge_out:output_FL
ototo_bt_bridge:monitor_FL
`
	stereoInputs = `ototo_bt_bridge:playback_FL
ototo_bt_bridge:playback_FR
bluez_output.00_11_22_33_44_55.1:playback_FL
bluez_output.00_11_22_33_44_55.1:playback_FR
`
	monoInputs = `ototo_bt_bridge:playback_FL
ototo_bt_bridge:playback_FR
bluez_output.00_11_22_33_44_55.1:playback_MONO
`
	// JamesDSP on the bridge, the bridge on the headset in A2DP.
	bridgedStereo = `jdsp_@PwJamesDspPlugin_JamesDsp:output_FL
  |-> ototo_bt_bridge:playback_FL
jdsp_@PwJamesDspPlugin_JamesDsp:output_FR
  |-> ototo_bt_bridge:playback_FR
ototo_bt_bridge_out:output_FL
  |-> bluez_output.00_11_22_33_44_55.1:playback_FL
ototo_bt_bridge_out:output_FR
  |-> bluez_output.00_11_22_33_44_55.1:playback_FR
`
	// The handsfree switch destroyed the headset node: the bridge's links went
	// with it.
	bridgedFloating = `jdsp_@PwJamesDspPlugin_JamesDsp:output_FL
  |-> ototo_bt_bridge:playback_FL
jdsp_@PwJamesDspPlugin_JamesDsp:output_FR
  |-> ototo_bt_bridge:playback_FR
ototo_bt_bridge_out:output_FL
ototo_bt_bridge_out:output_FR
`
	bridgedMono = `jdsp_@PwJamesDspPlugin_JamesDsp:output_FL
  |-> ototo_bt_bridge:playback_FL
jdsp_@PwJamesDspPlugin_JamesDsp:output_FR
  |-> ototo_bt_bridge:playback_FR
ototo_bt_bridge_out:output_FL
  |-> bluez_output.00_11_22_33_44_55.1:playback_MONO
ototo_bt_bridge_out:output_FR
  |-> bluez_output.00_11_22_33_44_55.1:playback_MONO
`
)

// scripted answers the three listings with the given text and records the
// calls that change the graph.
type scripted struct {
	outs, ins, links string
	changes          []string
}

func (s *scripted) run(_ context.Context, args ...string) (string, error) {
	switch strings.Join(args, " ") {
	case "-o":
		return s.outs, nil
	case "-i":
		return s.ins, nil
	case "-l":
		return s.links, nil
	}
	s.changes = append(s.changes, strings.Join(args, " "))
	return "", nil
}

// TestTheBridgeWiresAStereoHeadsetPairwise: in A2DP the bridge pairs FL with
// FL and FR with FR, after dropping whatever it was linked to.
func TestTheBridgeWiresAStereoHeadsetPairwise(t *testing.T) {
	s := &scripted{outs: bridgeOutputs, ins: stereoInputs, links: bridgedFloating}
	mono, err := NewWith(s.run).WireBridge(context.Background(), headset)
	require.NoError(t, err)
	require.False(t, mono)
	require.Equal(t, []string{
		"ototo_bt_bridge_out:output_FL bluez_output.00_11_22_33_44_55.1:playback_FL",
		"ototo_bt_bridge_out:output_FR bluez_output.00_11_22_33_44_55.1:playback_FR",
	}, s.changes)
}

// TestTheBridgeSumsIntoAHandsfreeHeadset: a handsfree sink has one mono port,
// and both channels go to it; linking only FL would drop the right channel.
func TestTheBridgeSumsIntoAHandsfreeHeadset(t *testing.T) {
	s := &scripted{outs: bridgeOutputs, ins: monoInputs, links: bridgedStereo}
	mono, err := NewWith(s.run).WireBridge(context.Background(), headset)
	require.NoError(t, err)
	require.True(t, mono, "the caller halves the level of a summed downmix")
	require.Equal(t, []string{
		"-d ototo_bt_bridge_out:output_FL bluez_output.00_11_22_33_44_55.1:playback_FL",
		"-d ototo_bt_bridge_out:output_FR bluez_output.00_11_22_33_44_55.1:playback_FR",
		"ototo_bt_bridge_out:output_FL bluez_output.00_11_22_33_44_55.1:playback_MONO",
		"ototo_bt_bridge_out:output_FR bluez_output.00_11_22_33_44_55.1:playback_MONO",
	}, s.changes)
}

// TestWiringAnAlreadyWiredBridgeChangesNothing: the wiring runs on every
// server event and tick, and an unlink drops audio, so correct links stay.
func TestWiringAnAlreadyWiredBridgeChangesNothing(t *testing.T) {
	for _, c := range []struct{ ins, links string }{{stereoInputs, bridgedStereo}, {monoInputs, bridgedMono}} {
		s := &scripted{outs: bridgeOutputs, ins: c.ins, links: c.links}
		_, err := NewWith(s.run).WireBridge(context.Background(), headset)
		require.NoError(t, err)
		require.Empty(t, s.changes)
	}
}

// TestWiringNamesWhatIsMissing: no bridge, or a headset with no ports, is a
// failure the caller reports, not a silent no-op.
func TestWiringNamesWhatIsMissing(t *testing.T) {
	s := &scripted{outs: outputs, ins: stereoInputs}
	_, err := NewWith(s.run).WireBridge(context.Background(), headset)
	require.ErrorIs(t, err, ErrNoBridge)
	s = &scripted{outs: bridgeOutputs, ins: inputs}
	_, err = NewWith(s.run).WireBridge(context.Background(), headset)
	require.ErrorIs(t, err, ErrNoInputs)
}

// TestTheTargetSeesThroughTheBridge: where JamesDSP plays is the hardware
// behind the bridge, so the auto-switch and the volume keys act on the
// headset, not on the bridge.
func TestTheTargetSeesThroughTheBridge(t *testing.T) {
	s := &scripted{outs: bridgeOutputs, links: bridgedMono}
	sink, via, err := NewWith(s.run).Route(context.Background())
	require.NoError(t, err)
	require.Equal(t, headset, sink)
	require.True(t, via)

	s.links = bridgedFloating
	sink, via, err = NewWith(s.run).Route(context.Background())
	require.NoError(t, err)
	require.Equal(t, "", sink, "a bridge linked to nothing is floating")
	require.True(t, via)

	direct := &scripted{outs: outputs, links: links}
	sink, via, err = NewWith(direct.run).Route(context.Background())
	require.NoError(t, err)
	require.Equal(t, "alsa_output.usb-Example_DAC-00.analog-stereo", sink)
	require.False(t, via)
}

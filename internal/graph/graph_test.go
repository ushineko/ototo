package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The shapes below are pw-link's, with invented sink names.
const (
	outputs = `Vivaldi:output_FL
Vivaldi:output_FR
jdsp_@PwJamesDspPlugin_JamesDsp:output_FR
jdsp_@PwJamesDspPlugin_JamesDsp:output_FL
jdsp_@PwJamesDspPlugin_JamesDsp:monitor_FL
alsa_input.usb-Example_DAC-00.analog-stereo:capture_FL
`
	inputs = `alsa_output.usb-Example_DAC-00.analog-stereo:playback_FR
alsa_output.usb-Example_DAC-00.analog-stereo:playback_FL
alsa_output.usb-Example_Headset-00.analog-stereo:playback_FL
alsa_output.usb-Example_Headset-00.analog-stereo:playback_FR
jdsp_@PwJamesDspPlugin_JamesDsp:input_FL
`
	links = `alsa_output.usb-Example_DAC-00.analog-stereo:playback_FL
  |<- jdsp_@PwJamesDspPlugin_JamesDsp:output_FL
Vivaldi:output_FL
  |-> jdsp_@PwJamesDspPlugin_JamesDsp:input_FL
jdsp_@PwJamesDspPlugin_JamesDsp:output_FL
  |-> alsa_output.usb-Example_DAC-00.analog-stereo:playback_FL
  |-> alsa_output.usb-Example_DAC-00.analog-stereo:monitor_FL
jdsp_@PwJamesDspPlugin_JamesDsp:output_FR
  |-> alsa_output.usb-Example_DAC-00.analog-stereo:playback_FR
alsa_output.usb-Example_Headset-00.analog-stereo:playback_FL
`
)

// TestOutputsAreTheFilterPortsSorted: the monitor port and the browser's
// ports are not JamesDSP outputs, and the sort is what pairs FL with FL.
func TestOutputsAreTheFilterPortsSorted(t *testing.T) {
	require.Equal(t, []string{
		"jdsp_@PwJamesDspPlugin_JamesDsp:output_FL",
		"jdsp_@PwJamesDspPlugin_JamesDsp:output_FR",
	}, parseOutputs(outputs))
	require.Empty(t, parseOutputs("Vivaldi:output_FL\n"))
}

// TestPlaybackPortsBelongToOneSink: a sink name that is a prefix of another
// must not collect the other's ports.
func TestPlaybackPortsBelongToOneSink(t *testing.T) {
	require.Equal(t, []string{
		"alsa_output.usb-Example_DAC-00.analog-stereo:playback_FL",
		"alsa_output.usb-Example_DAC-00.analog-stereo:playback_FR",
	}, parsePlayback(inputs, "alsa_output.usb-Example_DAC-00.analog-stereo"))
	require.Empty(t, parsePlayback(inputs, "alsa_output.usb-Nothing"))
}

// TestTheTargetIsReadFromTheOutgoingLinks: the |<- line under the sink's
// port is the same link seen from the other end and must not count twice,
// and the monitor link is not a playback link.
func TestTheTargetIsReadFromTheOutgoingLinks(t *testing.T) {
	require.Equal(t, []string{"alsa_output.usb-Example_DAC-00.analog-stereo:playback_FL"},
		linkedPorts(links, "jdsp_@PwJamesDspPlugin_JamesDsp:output_FL"))
	require.Equal(t, "alsa_output.usb-Example_DAC-00.analog-stereo",
		linkedSink(links, "jdsp_@PwJamesDspPlugin_JamesDsp:output_FL"))
	require.Equal(t, "", linkedSink(links, "jdsp_@PwJamesDspPlugin_JamesDsp:output_nope"))
}

// fake records every pw-link call and answers the listings from the fixtures.
type fake struct {
	calls [][]string
}

func (f *fake) run(_ context.Context, args ...string) (string, error) {
	f.calls = append(f.calls, args)
	switch strings.Join(args, " ") {
	case "-o":
		return outputs, nil
	case "-i":
		return inputs, nil
	case "-l":
		return links, nil
	}
	return "", nil
}

// TestRelinkUnlinksThenLinksPairwise: the old links go first, then FL to FL
// and FR to FR on the new sink, which is exclusive routing.
func TestRelinkUnlinksThenLinksPairwise(t *testing.T) {
	f := &fake{}
	g := NewWith(f.run)
	require.NoError(t, g.Relink(context.Background(), "alsa_output.usb-Example_Headset-00.analog-stereo"))
	var did []string
	for _, c := range f.calls {
		if len(c) > 1 {
			did = append(did, strings.Join(c, " "))
		}
	}
	require.Equal(t, []string{
		"-d jdsp_@PwJamesDspPlugin_JamesDsp:output_FL alsa_output.usb-Example_DAC-00.analog-stereo:playback_FL",
		"-d jdsp_@PwJamesDspPlugin_JamesDsp:output_FR alsa_output.usb-Example_DAC-00.analog-stereo:playback_FR",
		"jdsp_@PwJamesDspPlugin_JamesDsp:output_FL alsa_output.usb-Example_Headset-00.analog-stereo:playback_FL",
		"jdsp_@PwJamesDspPlugin_JamesDsp:output_FR alsa_output.usb-Example_Headset-00.analog-stereo:playback_FR",
	}, did)
}

// TestRelinkRefusesWithoutPorts: no filter, or a sink with no playback
// ports, is the failure that trips the breaker, and it must be named.
func TestRelinkRefusesWithoutPorts(t *testing.T) {
	g := NewWith(func(_ context.Context, args ...string) (string, error) {
		if args[0] == "-o" {
			return "Vivaldi:output_FL\n", nil
		}
		return inputs, nil
	})
	require.ErrorIs(t, g.Relink(context.Background(), "alsa_output.usb-Example_DAC-00.analog-stereo"), ErrNoOutputs)

	f := &fake{}
	require.ErrorIs(t, NewWith(f.run).Relink(context.Background(), "alsa_output.usb-Nothing"), ErrNoInputs)
}

// TestTargetOnTheLiveGraph runs where pw-link exists and only reads: the
// answer is whatever the machine has, and the check is that the parsers
// accept the real text.
func TestTargetOnTheLiveGraph(t *testing.T) {
	if !Available() {
		t.Skip("pw-link is not installed")
	}
	g := New()
	outs, err := g.JamesDSPOutputs(context.Background())
	if err != nil {
		t.Skipf("pw-link did not answer: %v", err)
	}
	target, err := g.Target(context.Background())
	require.NoError(t, err)
	if len(outs) == 0 {
		require.Equal(t, "", target)
	}
}

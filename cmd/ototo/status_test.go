package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/devices"
)

// TestStatusTextSaysWhyThereIsNoServer: the line a bug report needs is the
// reason, not a blank outputs list.
func TestStatusTextSaysWhyThereIsNoServer(t *testing.T) {
	var b strings.Builder
	printStatus(&b, core.StatusResult{Version: "1", Commit: "c", ServerError: "no socket"})
	out := b.String()
	require.Contains(t, out, "not reached (no socket)")
	require.Contains(t, out, "not written yet")
	require.NotContains(t, out, "devices:")
}

// TestStatusTextMarksTheDefaultOutput: the asterisk is the one thing a person
// scans the list for.
func TestStatusTextMarksTheDefaultOutput(t *testing.T) {
	var b strings.Builder
	printStatus(&b, core.StatusResult{
		Server: audio.Server{Name: "PulseAudio (on PipeWire 1.4.0)", DefaultSink: "a"},
		Devices: []devices.Device{
			{ID: "a", Name: "Speakers", Sink: "a", Online: true, Default: true, Connected: true, Volume: 50},
			{ID: "b", Name: "Headset [Disconnected]", Sink: "b", Online: true, Mute: true},
			{ID: "bt:AA:BB:CC:DD:EE:FF", Name: "AirPods [Disconnected]"},
		},
	})
	out := b.String()
	require.Contains(t, out, "* Speakers")
	require.Contains(t, out, "  Headset [Disconnected]")
	require.Contains(t, out, "muted")
	require.Contains(t, out, "away")
	require.Contains(t, out, "bt:AA:BB:CC:DD:EE:FF")
}

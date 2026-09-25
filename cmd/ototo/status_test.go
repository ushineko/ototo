package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/core"
)

// TestStatusTextSaysWhyThereIsNoServer: the line a bug report needs is the
// reason, not a blank outputs list.
func TestStatusTextSaysWhyThereIsNoServer(t *testing.T) {
	var b strings.Builder
	printStatus(&b, core.StatusResult{Version: "1", Commit: "c", ServerError: "no socket"})
	out := b.String()
	require.Contains(t, out, "not reached (no socket)")
	require.Contains(t, out, "not written yet")
	require.NotContains(t, out, "outputs:")
}

// TestStatusTextMarksTheDefaultOutput: the asterisk is the one thing a person
// scans the list for.
func TestStatusTextMarksTheDefaultOutput(t *testing.T) {
	var b strings.Builder
	printStatus(&b, core.StatusResult{
		Server: audio.Server{Name: "PulseAudio (on PipeWire 1.4.0)", DefaultSink: "a"},
		Outputs: []core.Output{
			{Name: "a", Description: "Speakers", Default: true, Connected: true, Volume: 50},
			{Name: "b", Description: "Headset", Mute: true},
		},
	})
	out := b.String()
	require.Contains(t, out, "* a")
	require.Contains(t, out, "  b")
	require.Contains(t, out, "disconnected")
	require.Contains(t, out, "muted")
}

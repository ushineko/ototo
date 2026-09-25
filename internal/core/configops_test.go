package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/config"
)

// TestMovedSwapsNeighboursAndStopsAtTheEnds: up from the top and down from
// the bottom are no-ops, and a device ranked for the first time joins at
// the end and moves from there.
func TestMovedSwapsNeighboursAndStopsAtTheEnds(t *testing.T) {
	order := []string{"a", "b", "c"}
	require.Equal(t, []string{"b", "a", "c"}, Moved(order, "b", -1))
	require.Equal(t, []string{"a", "c", "b"}, Moved(order, "b", +1))
	require.Equal(t, []string{"a", "b", "c"}, Moved(order, "a", -1))
	require.Equal(t, []string{"a", "b", "c"}, Moved(order, "c", +1))
	require.Equal(t, []string{"a", "b", "d", "c"}, Moved(order, "d", -1))
	require.Equal(t, []string{"a", "b", "c"}, order, "the input was changed in place")
}

// TestTheSettingsWritesLandInOneFile: every control writes the same
// document the flags read, and a nil switch leaves its key alone.
func TestTheSettingsWritesLandInOneFile(t *testing.T) {
	w := newWorld(t, false)
	ctx := context.Background()

	cfg, err := SetPriority(ctx, SetPriorityRequest{Request: w.req(), Order: []string{speakers, headsetSink}})
	require.NoError(t, err)
	require.Equal(t, []string{speakers, headsetSink}, cfg.DevicePriority)

	cfg, err = SetMicLink(ctx, SetMicLinkRequest{Request: w.req(), Device: speakers, Link: headMic})
	require.NoError(t, err)
	require.Equal(t, headMic, cfg.MicLinks[speakers])
	cfg, err = SetMicLink(ctx, SetMicLinkRequest{Request: w.req(), Device: speakers, Link: config.MicAuto})
	require.NoError(t, err)
	require.NotContains(t, cfg.MicLinks, speakers, "auto is the absence of a key")

	off := false
	cfg, err = SetSwitches(ctx, SetSwitchesRequest{Request: w.req(), OSDEnabled: &off})
	require.NoError(t, err)
	require.False(t, cfg.OSDEnabled)
	require.True(t, cfg.SwitchNotifications, "a switch that was not set changed")
	require.True(t, cfg.MoveStreams)

	saved, _, err := config.Load(w.path)
	require.NoError(t, err)
	require.Equal(t, cfg, saved)
}

// TestSetVolumeFollowsTheRouting: like the keys, the slider acts on the
// hardware sink behind JamesDSP, and mute is a separate write.
func TestSetVolumeFollowsTheRouting(t *testing.T) {
	w := newWorld(t, true)
	w.srv.defaultSink = "jamesdsp_sink"
	w.g.target = headsetSink
	res, err := w.sw.SetVolume(context.Background(), SetVolumeRequest{Request: w.req(), Percent: 30})
	require.NoError(t, err)
	require.Equal(t, headsetSink, res.Sink)
	require.Equal(t, 30, res.Percent)
	on := true
	res, err = w.sw.SetVolume(context.Background(), SetVolumeRequest{Request: w.req(), Mute: &on})
	require.NoError(t, err)
	require.True(t, res.Muted)
	require.Equal(t, 30, res.Percent, "mute changed the level")
}

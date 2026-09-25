package audio

import (
	"testing"

	"github.com/jfreymuth/pulse/proto"
	"github.com/stretchr/testify/require"
)

// TestVolumeIsClampedTo150: pactl accepts 300%, and a hotkey held down must
// not get there.
func TestVolumeIsClampedTo150(t *testing.T) {
	require.Equal(t, 0, clamp(-10))
	require.Equal(t, 150, clamp(151))
	require.Equal(t, 73, clamp(73))
}

// TestPercentRoundTripsThroughTheWireValue: what SetVolume writes, Percent
// reads back as the same number, for every step a key can land on.
func TestPercentRoundTripsThroughTheWireValue(t *testing.T) {
	for p := 0; p <= MaxVolumePercent; p += VolumeStep {
		require.Equal(t, p, Percent(proto.ChannelVolumes{fromPercent(p), fromPercent(p)}), "at %d%%", p)
	}
}

/*
TestTheWriteSideRoundTripsOnTheLiveServer runs where a server listens, and
leaves it as it found it: every write sets what is already set. That is the
one way to exercise the request encoding against a real server from a test
without turning the developer's speakers up.
*/
func TestTheWriteSideRoundTripsOnTheLiveServer(t *testing.T) {
	c := liveClient(t)
	srv, err := c.Server()
	require.NoError(t, err)
	if srv.DefaultSink == "" {
		t.Skip("the server has no default sink")
	}

	percent, muted, err := c.Volume(srv.DefaultSink)
	require.NoError(t, err)

	set, err := c.SetVolume(srv.DefaultSink, percent)
	require.NoError(t, err)
	require.Equal(t, percent, set)
	again, _, err := c.Volume(srv.DefaultSink)
	require.NoError(t, err)
	require.Equal(t, percent, again, "setting the current volume changed it")

	require.NoError(t, c.SetMute(srv.DefaultSink, muted))
	require.NoError(t, c.SetDefaultSink(srv.DefaultSink))
	if srv.DefaultSource != "" {
		require.NoError(t, c.SetDefaultSource(srv.DefaultSource))
	}

	inputs, err := c.SinkInputs()
	require.NoError(t, err)
	for _, in := range inputs {
		require.NotZero(t, in.Index+1)
	}
}

func liveClient(t *testing.T) *Client {
	t.Helper()
	skipWithoutServer(t)
	c, err := Connect("")
	if err != nil {
		t.Skipf("no sound server answered: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

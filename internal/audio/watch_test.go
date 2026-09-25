package audio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jfreymuth/pulse/proto"
	"github.com/stretchr/testify/require"
)

func skipWithoutServer(t *testing.T) {
	t.Helper()
	if os.Getenv("PULSE_SERVER") != "" {
		return
	}
	sock := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "pulse", "native")
	if _, err := os.Stat(sock); err != nil {
		t.Skipf("no sound server socket at %s", sock)
	}
}

// TestTranslateKeepsOnlyWhatTheSwitcherListensFor: a module or client event
// is noise to a program that watches sinks, and the string form of the
// original's `pactl subscribe` filter matched "sink" alone.
func TestTranslateKeepsOnlyWhatTheSwitcherListensFor(t *testing.T) {
	ev, ok := translate(&proto.SubscribeEvent{Event: proto.EventSink | proto.EventChange, Index: 7})
	require.True(t, ok)
	require.Equal(t, Event{Facility: FacilitySink, Change: ChangeChanged, Index: 7}, ev)

	ev, ok = translate(&proto.SubscribeEvent{Event: proto.EventCard | proto.EventRemove, Index: 2})
	require.True(t, ok)
	require.Equal(t, Event{Facility: FacilityCard, Change: ChangeRemoved, Index: 2}, ev)

	_, ok = translate(&proto.SubscribeEvent{Event: proto.EventModule | proto.EventNew})
	require.False(t, ok, "a module event reached the switcher")
	_, ok = translate(&proto.SubscribeEvent{Event: proto.EventClient | proto.EventChange})
	require.False(t, ok, "a client event reached the switcher")
}

// TestWatchStopsWithTheContextWhenThereIsNoServer: a machine with no server
// must not keep a goroutine retrying after the program asked it to stop, and
// the events channel must close so a receiver's range ends.
func TestWatchStopsWithTheContextWhenThereIsNoServer(t *testing.T) {
	t.Setenv("PULSE_SERVER", "unix:"+filepath.Join(t.TempDir(), "no-server"))
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event)
	var logged []string
	done := make(chan struct{})
	go func() {
		Watch(ctx, "", events, func(s string) { logged = append(logged, s) })
		close(done)
	}()
	time.Sleep(600 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after cancel")
	}
	_, open := <-events
	require.False(t, open, "the events channel stayed open")
	require.NotEmpty(t, logged, "the failure to connect went unreported")
}

/*
TestWatchSeesAVolumeWriteOnTheLiveServer subscribes, nudges the default sink's
volume by one point and puts it back, and expects a sink change event: the
write is what the indicator listens for, and a subscription that misses it is
an indicator that never appears.

The nudge is real, because PipeWire reports no change for a write that changes
nothing. One point for a few milliseconds is below hearing, and Cleanup
restores the value whatever happens. The write is repeated until an event
arrives, because the subscription lands asynchronously and a write that
precedes it is not an event.
*/
func TestWatchSeesAVolumeWriteOnTheLiveServer(t *testing.T) {
	c := liveClient(t)
	srv, err := c.Server()
	require.NoError(t, err)
	if srv.DefaultSink == "" {
		t.Skip("the server has no default sink")
	}
	percent, _, err := c.Volume(srv.DefaultSink)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = c.SetVolume(srv.DefaultSink, percent) })
	nudged := percent - 1
	if percent == 0 {
		nudged = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	events := make(chan Event, 64)
	go Watch(ctx, "", events, nil)

	for {
		_, err = c.SetVolume(srv.DefaultSink, nudged)
		require.NoError(t, err)
		_, err = c.SetVolume(srv.DefaultSink, percent)
		require.NoError(t, err)
		wait := time.After(500 * time.Millisecond)
	drain:
		for {
			select {
			case ev := <-events:
				if ev.Facility == FacilitySink && ev.Change == ChangeChanged {
					return
				}
			case <-wait:
				break drain
			case <-ctx.Done():
				t.Fatal("no sink change event arrived after a volume write")
			}
		}
	}
}

package core

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/loopback"
)

type fakeLoop struct {
	service bool
	started []string
}

func (f *fakeLoop) run(_ context.Context, _ string, args ...string) (string, error) {
	if strings.HasSuffix(strings.Join(args, " "), "cat "+loopback.ServiceName) && !f.service {
		return "", context.Canceled
	}
	return "inactive", nil
}

func (f *fakeLoop) start(name string, args ...string) (func(), func() bool, error) {
	f.started = append(f.started, name+" "+strings.Join(args, " "))
	return func() {}, func() bool { return true }, nil
}

func lineIn(w *world) {
	w.srv.sources = append(w.srv.sources, audio.Device{
		Name: "alsa_input.usb-Example_DAC-00.analog-stereo-linein", ActivePort: "analog-input-linein",
		Ports: []audio.Port{{Name: "analog-input-linein", Description: "Line In"}},
	})
}

// TestSetLoopbackStartsTheChildTowardJamesDSPAndRemembers: the direct tier
// targets the JamesDSP sink when there is one, and the choice lands in the
// settings so the next start restores it.
func TestSetLoopbackStartsTheChildTowardJamesDSPAndRemembers(t *testing.T) {
	w := newWorld(t, true)
	lineIn(w)
	f := &fakeLoop{}
	w.sw.Loopback = loopback.NewWith(f.run, f.start)
	st, err := w.sw.SetLoopback(context.Background(), SetLoopbackRequest{Request: w.req(), Enabled: true})
	require.NoError(t, err)
	require.True(t, st.Active)
	require.Equal(t, loopback.ModeDirect, st.Mode)
	require.Equal(t, []string{"pw-loopback -C alsa_input.usb-Example_DAC-00.analog-stereo-linein -P jamesdsp_sink"}, f.started)
	cfg, _, err := config.Load(w.path)
	require.NoError(t, err)
	require.True(t, cfg.LoopbackEnabled)

	// A fresh process restores it.
	f2 := &fakeLoop{}
	w.sw.Loopback = loopback.NewWith(f2.run, f2.start)
	st, err = w.sw.RestoreLoopback(context.Background(), LoopbackRequest{Request: w.req()})
	require.NoError(t, err)
	require.True(t, st.Active)
	require.Len(t, f2.started, 1)

	// With the unit installed, systemd remembers and nothing is spawned.
	f3 := &fakeLoop{service: true}
	w.sw.Loopback = loopback.NewWith(f3.run, f3.start)
	st, err = w.sw.RestoreLoopback(context.Background(), LoopbackRequest{Request: w.req()})
	require.NoError(t, err)
	require.Equal(t, loopback.ModeService, st.Mode)
	require.Empty(t, f3.started)
}

// TestNoLineInMeansNoLoopback: the state says so and Set refuses.
func TestNoLineInMeansNoLoopback(t *testing.T) {
	w := newWorld(t, false)
	f := &fakeLoop{}
	w.sw.Loopback = loopback.NewWith(f.run, f.start)
	st, err := w.sw.LoopbackState(context.Background(), LoopbackRequest{Request: w.req()})
	require.NoError(t, err)
	require.Equal(t, loopback.ModeNone, st.Mode)
	_, err = w.sw.SetLoopback(context.Background(), SetLoopbackRequest{Request: w.req(), Enabled: true})
	require.ErrorIs(t, err, loopback.ErrNoLineIn)
}

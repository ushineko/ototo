package loopback

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
)

func source(name, port, desc string) audio.Device {
	return audio.Device{Name: name, ActivePort: port, Ports: []audio.Port{{Name: port, Description: desc}}}
}

// TestTheLineInIsFoundByItsPort: the monitor of an output is not an input,
// a microphone is not a line-in, and the port is judged by its active port
// only.
func TestTheLineInIsFoundByItsPort(t *testing.T) {
	sources := []audio.Device{
		source("alsa_output.usb-Example-00.analog-stereo.monitor", "analog-output-lineout", "Line Out"),
		source("alsa_input.usb-Example-00.mono", "analog-input-mic", "Microphone"),
		source("alsa_input.usb-Example-00.analog-stereo-linein", "analog-input-linein", "Line In"),
	}
	require.Equal(t, "alsa_input.usb-Example-00.analog-stereo-linein", LineInSource(sources))
	require.Equal(t, "", LineInSource(sources[:2]))
	inactive := source("alsa_input.x", "analog-input-mic", "Microphone")
	inactive.Ports = append(inactive.Ports, audio.Port{Name: "analog-input-linein", Description: "Line In"})
	require.Equal(t, "", LineInSource([]audio.Device{inactive}), "an inactive line-in port counted")
}

// TestPickHonoursTheSettingWhenItIsPresent: with two line inputs the
// setting chooses; a setting that names a source that is away falls back
// to the first rather than failing.
func TestPickHonoursTheSettingWhenItIsPresent(t *testing.T) {
	sources := []audio.Device{
		source("alsa_input.a-linein", "analog-input-linein", "Line In"),
		source("alsa_input.b-linein", "analog-input-linein", "Line In"),
	}
	got, cands := Pick(sources, "alsa_input.b-linein")
	require.Equal(t, "alsa_input.b-linein", got)
	require.Equal(t, []string{"alsa_input.a-linein", "alsa_input.b-linein"}, cands)
	got, _ = Pick(sources, "alsa_input.gone")
	require.Equal(t, "alsa_input.a-linein", got)
	got, _ = Pick(sources, "")
	require.Equal(t, "alsa_input.a-linein", got)
}

func TestTheTargetIsJamesDSPWhenPresent(t *testing.T) {
	require.Equal(t, "@DEFAULT_SINK@", TargetSink([]audio.Device{{Name: "alsa_output.x"}}))
	require.Equal(t, "jamesdsp_sink", TargetSink([]audio.Device{{Name: "alsa_output.x"}, {Name: "jamesdsp_sink"}}))
}

type fake struct {
	service bool
	active  bool
	calls   []string
	started []string
	stopped int
}

func (f *fake) run(_ context.Context, name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	switch {
	case strings.HasSuffix(call, "cat "+ServiceName):
		if !f.service {
			return "", errors.New("no such unit")
		}
		return "[Unit]", nil
	case strings.HasSuffix(call, "is-active "+ServiceName):
		if f.active {
			return "active\n", nil
		}
		return "inactive\n", errors.New("exit 3")
	case strings.HasSuffix(call, "start "+ServiceName):
		f.active = true
	case strings.HasSuffix(call, "stop "+ServiceName):
		f.active = false
	}
	return "", nil
}

func (f *fake) start(name string, args ...string) (func(), func() bool, error) {
	f.started = append(f.started, name+" "+strings.Join(args, " "))
	alive := true
	return func() { alive = false; f.stopped++ }, func() bool { return alive }, nil
}

// TestTheServiceTierIsUsedWhenTheUnitExists: with the unit installed,
// nothing is spawned; the unit is started and stopped.
func TestTheServiceTierIsUsedWhenTheUnitExists(t *testing.T) {
	f := &fake{service: true}
	l := NewWith(f.run, f.start)
	ctx := context.Background()
	st := l.State(ctx, "src")
	require.Equal(t, ModeService, st.Mode)
	require.False(t, st.Active)
	st, err := l.Set(ctx, true, "src", "jamesdsp_sink")
	require.NoError(t, err)
	require.True(t, st.Active)
	require.Empty(t, f.started, "pw-loopback was started beside the unit")
	st, err = l.Set(ctx, false, "src", "jamesdsp_sink")
	require.NoError(t, err)
	require.False(t, st.Active)
}

// TestTheDirectTierOwnsOneChild: a second Set replaces the child rather
// than adding one, off stops it, and Close is safe twice.
func TestTheDirectTierOwnsOneChild(t *testing.T) {
	f := &fake{}
	l := NewWith(f.run, f.start)
	ctx := context.Background()
	require.Equal(t, ModeDirect, l.State(ctx, "src").Mode)
	st, err := l.Set(ctx, true, "src", "jamesdsp_sink")
	require.NoError(t, err)
	require.True(t, st.Active)
	require.Equal(t, []string{"pw-loopback -C src -P jamesdsp_sink"}, f.started)
	require.True(t, l.State(ctx, "src").Active)
	_, err = l.Set(ctx, true, "src", "jamesdsp_sink")
	require.NoError(t, err)
	require.Equal(t, 1, f.stopped, "the first child was not stopped before the second")
	require.Len(t, f.started, 2)
	_, err = l.Set(ctx, false, "src", "jamesdsp_sink")
	require.NoError(t, err)
	require.False(t, l.State(ctx, "src").Active)
	l.Close()
	l.Close()
	require.Equal(t, 2, f.stopped)
}

func TestNoLineInIsARefusal(t *testing.T) {
	l := NewWith((&fake{}).run, (&fake{}).start)
	_, err := l.Set(context.Background(), true, "", "x")
	require.ErrorIs(t, err, ErrNoLineIn)
	require.Equal(t, ModeNone, l.State(context.Background(), "").Mode)
}

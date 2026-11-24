package bridge

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// child is a stand-in for pw-loopback: it counts starts and can be killed.
type child struct {
	starts  atomic.Int32
	alive   atomic.Bool
	stopped atomic.Int32
	fail    error
	args    []string
}

func (c *child) start(name string, args ...string) (func(), func() bool, error) {
	if c.fail != nil {
		return nil, nil, c.fail
	}
	c.starts.Add(1)
	c.alive.Store(true)
	c.args = append([]string{name}, args...)
	return func() { c.alive.Store(false); c.stopped.Add(1) }, c.alive.Load, nil
}

func always(context.Context) bool { return true }

// TestTheBridgeStartsOnceAndRestartsWhenDead: repeated routes share one
// child, and a child that died is started again on the next route. Each
// start is reported as fresh, because a new child has never been wired to
// anything (spec 007 D2).
func TestTheBridgeStartsOnceAndRestartsWhenDead(t *testing.T) {
	c := &child{}
	b := NewWith(c.start)
	fresh, err := b.Ensure(context.Background(), always)
	require.NoError(t, err)
	require.True(t, fresh)
	fresh, err = b.Ensure(context.Background(), always)
	require.NoError(t, err)
	require.False(t, fresh, "the running child was reported as new")
	require.EqualValues(t, 1, c.starts.Load())

	c.alive.Store(false)
	fresh, err = b.Ensure(context.Background(), always)
	require.NoError(t, err)
	require.True(t, fresh, "a restarted child was taken for the old one")
	require.EqualValues(t, 2, c.starts.Load())
}

// ensure is Ensure for a test that does not care whether the child is new.
func ensure(ctx context.Context, b *Bridge, ready func(context.Context) bool) error {
	_, err := b.Ensure(ctx, ready)
	return err
}

// TestCloseLeavesNoChild: the process exit stops the child, and a second
// Close does nothing.
func TestCloseLeavesNoChild(t *testing.T) {
	c := &child{}
	b := NewWith(c.start)
	require.NoError(t, ensure(context.Background(), b, always))
	b.Close()
	b.Close()
	require.False(t, c.alive.Load())
	require.EqualValues(t, 1, c.stopped.Load())
	require.False(t, b.Running())
}

// TestABridgeThatNeverAppearsIsStoppedAndReported: the caller falls back to
// the direct route, and no half-started child is left behind.
func TestABridgeThatNeverAppearsIsStoppedAndReported(t *testing.T) {
	c := &child{}
	b := NewWith(c.start)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ensure(ctx, b, func(context.Context) bool { return false })
	require.ErrorIs(t, err, ErrNotReady)
	require.False(t, c.alive.Load())
}

// TestAMissingProgramIsAnError: no pw-loopback is the failure R1.4 reports.
func TestAMissingProgramIsAnError(t *testing.T) {
	c := &child{fail: errors.New("exec: not found")}
	require.Error(t, ensure(context.Background(), NewWith(c.start), always))
}

// TestThePlaybackSideIsNotAutoconnected: with autoconnect on, WirePlumber
// links the bridge into JamesDSP's own sink, a feedback loop.
func TestThePlaybackSideIsNotAutoconnected(t *testing.T) {
	c := &child{}
	require.NoError(t, ensure(context.Background(), NewWith(c.start), always))
	joined := strings.Join(c.args, " ")
	require.Contains(t, joined, "node.name=ototo_bt_bridge_out node.autoconnect=false")
	require.Contains(t, joined, "media.class=Audio/Sink audio.position=[FL,FR]")
	require.Equal(t, "pw-loopback", c.args[0])
}

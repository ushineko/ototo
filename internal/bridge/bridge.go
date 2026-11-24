/*
Package bridge runs the stereo bridge JamesDSP plays into on the way to a
Bluetooth sink (spec 006 R1).

JamesDSP outputs silence into a mono handsfree sink (JDSP4Linux #298), which
WirePlumber up to 0.5.13 hid behind a stereo loopback of its own and 0.5.14
stopped creating. The bridge puts that loopback back: a pw-loopback child of
the resident process whose capture side is a stereo sink and whose playback
side is not connected by the session manager, so the graph package links it
to the headset and nothing else moves it. The child lives from the first
Bluetooth route until the process exits.
*/
package bridge

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ushineko/ototo/internal/graph"
	"github.com/ushineko/ototo/internal/loopback"
)

// ReadyTimeout bounds the wait for a new child's nodes to appear in the
// graph; pw-loopback registers them in well under a second.
const ReadyTimeout = 3 * time.Second

// readyPoll is how often the wait looks again.
const readyPoll = 100 * time.Millisecond

// MonoLevel is the bridge stream's gain while it feeds a mono handsfree
// port: both channels are summed into that port, and half of each is an
// (L+R)/2 downmix that does not clip (-6 dB).
const MonoLevel = 0.5

// ErrNotReady is a child that started but whose nodes never appeared.
var ErrNotReady = errors.New("the Bluetooth bridge did not appear in the graph")

/*
Args are pw-loopback's arguments. The playback side has autoconnect off:
with it on, WirePlumber ignores a target and links the stream into the
default sink, which is JamesDSP's own, a feedback loop. No value has a
space in it: pw-loopback exits at once on a quoted value with spaces when
started outside a shell.
*/
func Args() []string {
	return []string{
		"-n", "ototo-bt-bridge",
		"--capture-props=node.name=" + graph.BridgeSink +
			" node.description=ototo-bluetooth-bridge media.class=Audio/Sink audio.position=[FL,FR]",
		"--playback-props=node.name=" + graph.BridgeOut + " node.autoconnect=false",
	}
}

// Bridge holds the child, when there is one.
type Bridge struct {
	start loopback.Starter

	mu      sync.Mutex
	stop    func()
	running func() bool
}

// New uses the real pw-loopback.
func New() *Bridge { return &Bridge{start: loopback.StartCommand} }

// NewWith uses the given starter, for tests.
func NewWith(start loopback.Starter) *Bridge { return &Bridge{start: start} }

/*
Ensure starts the child unless it runs, and waits until ready reports its
nodes in the graph. A child that has died since the last call is started
again (R1.2); one that never becomes ready is stopped, so a later call
starts clean. fresh says a new child started: its nodes have never been
wired to anything, which spec 007 D2 needs to know.
*/
func (b *Bridge) Ensure(ctx context.Context, ready func(context.Context) bool) (fresh bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running != nil && b.running() {
		return false, nil
	}
	stop, running, err := b.start("pw-loopback", Args()...)
	if err != nil {
		return false, fmt.Errorf("start pw-loopback: %w", err)
	}
	b.stop, b.running = stop, running

	deadline := time.Now().Add(ReadyTimeout)
	for {
		if ready(ctx) {
			return true, nil
		}
		if !running() || time.Now().After(deadline) || ctx.Err() != nil {
			b.stopLocked()
			return false, ErrNotReady
		}
		time.Sleep(readyPoll)
	}
}

// Running reports whether the child runs.
func (b *Bridge) Running() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running != nil && b.running()
}

// Close stops the child, if any (R1.3). Safe to call more than once.
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopLocked()
}

func (b *Bridge) stopLocked() {
	if b.stop != nil {
		b.stop()
		b.stop, b.running = nil, nil
	}
}

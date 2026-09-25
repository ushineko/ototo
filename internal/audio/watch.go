package audio

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jfreymuth/pulse/proto"
)

// Facility is what an event is about.
type Facility int

// Facilities the watcher reports. Anything else is dropped.
const (
	FacilitySink Facility = iota
	FacilitySource
	FacilitySinkInput
	FacilityServer
	FacilityCard
)

func (f Facility) String() string {
	switch f {
	case FacilitySink:
		return "sink"
	case FacilitySource:
		return "source"
	case FacilitySinkInput:
		return "sink-input"
	case FacilityServer:
		return "server"
	case FacilityCard:
		return "card"
	default:
		return "?"
	}
}

// Change is what happened to it.
type Change int

// Changes the server reports.
const (
	ChangeNew Change = iota
	ChangeChanged
	ChangeRemoved
)

func (c Change) String() string {
	switch c {
	case ChangeNew:
		return "new"
	case ChangeChanged:
		return "change"
	case ChangeRemoved:
		return "remove"
	default:
		return "?"
	}
}

// Event is one change the server reported.
type Event struct {
	Facility Facility
	Change   Change
	Index    uint32
	// Reconnected marks the synthetic event sent after the watcher lost the
	// server and found it again. Everything may have changed; read it all.
	Reconnected bool
}

// Backoff bounds for the reconnection loop. The first retry is quick, because
// a server restart is a second or two; the ceiling keeps a machine with no
// server from spinning.
const (
	backoffFirst = 250 * time.Millisecond
	backoffMax   = 5 * time.Second
)

/*
Watch subscribes to the server's change events and sends them on events until
ctx ends. It closes events when it returns.

It survives the server: when the connection drops, it reconnects with backoff
and sends one Event with Reconnected set, so the receiver knows to read the
whole state again rather than trust the last snapshot. log receives what
happened on the way and may be nil.

The original ran `pactl subscribe` in a thread and matched lines of text; this
is the same subscription over the protocol, and the events carry the facility
and index rather than a sentence.
*/
func Watch(ctx context.Context, server string, events chan<- Event, log func(string)) {
	defer close(events)
	say := func(format string, args ...any) {
		if log != nil {
			log(fmt.Sprintf(format, args...))
		}
	}
	backoff := backoffFirst
	first := true
	for {
		err := watchOnce(ctx, server, events, !first)
		if ctx.Err() != nil {
			return
		}
		first = false
		// A connection that was up and dropped is a server restarting, so the
		// next try is quick; a connection that never came up backs off.
		if errors.Is(err, errClosed) {
			say("the sound server went away; reconnecting")
			backoff = backoffFirst
		} else {
			say("sound server subscription: %v; retrying in %s", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, backoffMax)
	}
}

var errClosed = errors.New("the sound server closed the connection")

// watchOnce holds one subscription until the connection ends or ctx does.
// reconnected says whether to announce the reconnection first.
func watchOnce(ctx context.Context, server string, events chan<- Event, reconnected bool) error {
	c, err := Connect(server)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	closed := make(chan struct{})
	incoming := make(chan Event, 64)
	// The callback runs on the client's read goroutine. It is set before the
	// Subscribe request, and the request's reply is dispatched under the
	// client's lock, so the read goroutine sees the assignment before it can
	// see an event.
	c.c.Callback = func(m any) {
		switch m := m.(type) {
		case *proto.SubscribeEvent:
			if ev, ok := translate(m); ok {
				select {
				case incoming <- ev:
				default: // a receiver that is behind loses the oldest, not the connection
				}
			}
		case *proto.ConnectionClosed:
			close(closed)
		}
	}
	mask := proto.SubscriptionMaskSink | proto.SubscriptionMaskSource | proto.SubscriptionMaskSinkInput |
		proto.SubscriptionMaskServer | proto.SubscriptionMaskCard
	if err := c.c.Request(&proto.Subscribe{Mask: mask}, nil); err != nil {
		return fmt.Errorf("subscribe to the sound server: %w", err)
	}

	if reconnected {
		select {
		case events <- Event{Facility: FacilityServer, Change: ChangeChanged, Reconnected: true}:
		case <-ctx.Done():
			return nil
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-closed:
			return errClosed
		case ev := <-incoming:
			select {
			case events <- ev:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func translate(m *proto.SubscribeEvent) (Event, bool) {
	var ev Event
	switch m.Event.GetFacility() {
	case proto.EventSink:
		ev.Facility = FacilitySink
	case proto.EventSource:
		ev.Facility = FacilitySource
	case proto.EventSinkSinkInput:
		ev.Facility = FacilitySinkInput
	case proto.EventServer:
		ev.Facility = FacilityServer
	case proto.EventCard:
		ev.Facility = FacilityCard
	default:
		return Event{}, false
	}
	switch m.Event.GetType() {
	case proto.EventNew:
		ev.Change = ChangeNew
	case proto.EventChange:
		ev.Change = ChangeChanged
	case proto.EventRemove:
		ev.Change = ChangeRemoved
	default:
		return Event{}, false
	}
	ev.Index = m.Index
	return ev, true
}

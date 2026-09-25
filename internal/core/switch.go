package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/devices"
	"github.com/ushineko/ototo/internal/graph"
	"github.com/ushineko/ototo/internal/notify"
)

// server is what switching needs from the sound server. *audio.Client is
// one; the tests supply another.
type server interface {
	Server() (audio.Server, error)
	Sinks() ([]audio.Device, error)
	Sources() ([]audio.Device, error)
	SetDefaultSink(name string) error
	SetDefaultSource(name string) error
	MoveSinkInputs(sink string) (moved, refused int, err error)
	Volume(sink string) (percent int, muted bool, err error)
	AdjustVolume(sink string, delta int) (int, error)
	Close() error
}

func dialAudio(name string) (server, error) { return audio.Connect(name) }

/*
Switcher is the switching state the original kept on its window across
ticks: the JamesDSP circuit breaker and the last physical sink, which decides
whether a switch is worth a notification.

The resident process holds one for its lifetime. A one-shot --connect makes a
fresh one, as the original's headless mode did, so it starts with the breaker
closed and always notifies.
*/
type Switcher struct {
	// Graph is the PipeWire graph; nil uses pw-link on PATH.
	Graph *graph.Graph
	// Notifier receives the notifications; nil discards them.
	Notifier notify.Notifier

	dial func(server string) (server, error)

	mu           sync.Mutex
	jdspBroken   bool
	lastPhysical string
}

// NewSwitcher is a Switcher over the real server and graph.
func NewSwitcher() *Switcher {
	return &Switcher{dial: dialAudio}
}

func (s *Switcher) graph() *graph.Graph {
	if s.Graph == nil {
		s.Graph = graph.New()
	}
	return s.Graph
}

func (s *Switcher) notifier() notify.Notifier {
	if s.Notifier == nil {
		return notify.Discard{}
	}
	return s.Notifier
}

func (s *Switcher) connect(name string) (server, error) {
	if s.dial == nil {
		s.dial = dialAudio
	}
	return s.dial(name)
}

// JamesDSPBroken reports whether the breaker is open: JamesDSP was found and
// could not be rewired, so switches go to the hardware until something
// closes it (R5.4).
func (s *Switcher) JamesDSPBroken() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jdspBroken
}

// SwitchRequest names the output to switch to.
type SwitchRequest struct {
	Request
	// Target is a priority id, a sink name, or a case-insensitive substring
	// of a device's name (R5.5).
	Target string
	// Manual marks a switch the user asked for, which closes the breaker
	// first: a person clicking a device is the reset the original offered.
	Manual bool
}

// SwitchResult is what a switch did.
type SwitchResult struct {
	// Device is the row that was switched to.
	Device devices.Device
	// ViaJamesDSP is true when the default is the JamesDSP sink and its
	// output was rewired to Device.
	ViaJamesDSP bool
	// Fallback is why JamesDSP was bypassed after being found: the relink
	// failed and the breaker is now open.
	Fallback string
	// Mic is the input that now follows the output, "" when unchanged.
	Mic     string
	MicName string
	// StreamsMoved and StreamsRefused count the playing streams.
	StreamsMoved   int
	StreamsRefused int
	// Notified says whether "Audio Switched" was sent.
	Notified bool
}

// ErrNotFound and ErrNotConnected are why a target could not be switched to.
var (
	ErrNotFound     = errors.New("no device matches")
	ErrNotConnected = errors.New("the device has no sink right now")
)

// Switch makes the target the output (R5.1, R5.2, R5.5). A failure is
// notified as well as returned, because a hotkey has no terminal.
func (s *Switcher) Switch(ctx context.Context, req SwitchRequest) (SwitchResult, error) {
	res, err := s.switchByName(ctx, req)
	if err != nil {
		_, _ = s.notifier().Send(notify.Notification{
			Title: "Switch Failed", Body: err.Error(), Icon: notify.IconFailure,
		})
	}
	return res, err
}

func (s *Switcher) switchByName(ctx context.Context, req SwitchRequest) (SwitchResult, error) {
	cfg, _, err := config.Load(req.ConfigPath)
	if err != nil {
		return SwitchResult{}, err
	}
	srv, err := s.connect(req.Server)
	if err != nil {
		return SwitchResult{}, err
	}
	defer func() { _ = srv.Close() }()

	probes := req.probes()
	in := devices.Inputs{
		Priority:  cfg.DevicePriority,
		Bluetooth: probes.bluetooth(ctx),
		Headset:   probes.headset(ctx),
	}
	list, err := s.list(srv, in)
	if err != nil {
		return SwitchResult{}, err
	}
	dev, ok := Resolve(list, req.Target)
	if !ok {
		return SwitchResult{}, fmt.Errorf("%w %q", ErrNotFound, req.Target)
	}
	if !dev.Online {
		if dev.MAC == "" {
			return SwitchResult{}, fmt.Errorf("%w: %s", ErrNotConnected, dev.Name)
		}
		dev, err = s.connectAndWait(ctx, srv, in, probes, dev, req.Events)
		if err != nil {
			return SwitchResult{}, err
		}
	}
	if req.Manual {
		s.mu.Lock()
		s.jdspBroken = false
		s.mu.Unlock()
	}
	return s.switchTo(ctx, srv, cfg, dev, req.Events)
}

// list reads the sinks and builds the device list.
func (s *Switcher) list(srv server, in devices.Inputs) ([]devices.Device, error) {
	info, err := srv.Server()
	if err != nil {
		return nil, err
	}
	sinks, err := srv.Sinks()
	if err != nil {
		return nil, err
	}
	in.Sinks, in.DefaultSink = sinks, info.DefaultSink
	return devices.List(in), nil
}

// How long a Bluetooth device is given to produce a sink after Connect:
// twenty polls half a second apart, as the original.
const (
	connectPoll     = 500 * time.Millisecond
	connectAttempts = 20
)

// ErrConnectTimeout is a device that BlueZ connected but the sound server
// never showed.
var ErrConnectTimeout = errors.New("the device connected but no sink appeared")

/*
connectAndWait is R5.5's offline branch: ask BlueZ to connect the device,
then poll the sinks until one with the device's id appears, and return that
row. "Connecting..." goes out first, because a Bluetooth connect takes
seconds and the person who pressed the key would otherwise press it again.
*/
func (s *Switcher) connectAndWait(ctx context.Context, srv server, in devices.Inputs, probes Probes,
	dev devices.Device, ev Events) (devices.Device, error) {
	if probes.Connect == nil {
		return dev, fmt.Errorf("%w: %s, and there is no Bluetooth adapter to connect it with", ErrNotConnected, dev.Name)
	}
	_, _ = s.notifier().Send(notify.Notification{Title: "Connecting...", Body: "Connecting to " + dev.Name})
	ev.logf(LevelInfo, "connecting %s (%s)", dev.Name, dev.MAC)
	if err := probes.Connect(ctx, dev.MAC); err != nil {
		return dev, fmt.Errorf("connection failed: %w", err)
	}
	for range connectAttempts {
		select {
		case <-ctx.Done():
			return dev, fmt.Errorf("waiting for %s: %w", dev.Name, ctx.Err())
		case <-time.After(connectPoll):
		}
		list, err := s.list(srv, in)
		if err != nil {
			return dev, err
		}
		for _, d := range list {
			if d.ID == dev.ID && d.Online {
				return d, nil
			}
		}
	}
	return dev, fmt.Errorf("%w: %s", ErrConnectTimeout, dev.Name)
}

/*
Resolve finds the device a name refers to, in the original's order: the
exact priority id, then a case-insensitive substring of the display name,
the sink name or the id. The first match in list order wins, and the list is
in priority order, so a substring that matches two devices picks the one
the user ranks higher.
*/
func Resolve(list []devices.Device, target string) (devices.Device, bool) {
	for _, d := range list {
		if d.ID == target {
			return d, true
		}
	}
	needle := strings.ToLower(target)
	if needle == "" {
		return devices.Device{}, false
	}
	for _, d := range list {
		if strings.Contains(strings.ToLower(d.Name), needle) ||
			strings.Contains(strings.ToLower(d.Sink), needle) ||
			strings.Contains(strings.ToLower(d.ID), needle) {
			return d, true
		}
	}
	return devices.Device{}, false
}

// switchTo is R5.1 and R5.2 for a device that has a sink.
func (s *Switcher) switchTo(ctx context.Context, srv server, cfg config.Config, dev devices.Device, ev Events) (SwitchResult, error) {
	res := SwitchResult{Device: dev}

	useJDSP := false
	if dev.Sink != devices.JamesDSPSink && !s.JamesDSPBroken() {
		outs, err := s.graph().JamesDSPOutputs(ctx)
		if err != nil && !errors.Is(err, graph.ErrNotAvailable) {
			ev.logf(LevelWarn, "reading the PipeWire graph: %v", err)
		}
		useJDSP = len(outs) > 0
	}

	move := func(sink string) {
		if !cfg.MoveStreams {
			return
		}
		moved, refused, err := srv.MoveSinkInputs(sink)
		if err != nil {
			ev.logf(LevelWarn, "moving streams to %s: %v", sink, err)
			return
		}
		res.StreamsMoved, res.StreamsRefused = moved, refused
	}

	if useJDSP {
		ev.logf(LevelInfo, "JamesDSP found; rewiring it to %s", dev.Sink)
		if err := srv.SetDefaultSink(devices.JamesDSPSink); err != nil {
			return res, err
		}
		move(devices.JamesDSPSink)
		if err := s.graph().Relink(ctx, dev.Sink); err != nil {
			ev.logf(LevelWarn, "JamesDSP could not be rewired (%v); switching the hardware directly", err)
			s.mu.Lock()
			s.jdspBroken = true
			s.mu.Unlock()
			res.Fallback = err.Error()
			if err := srv.SetDefaultSink(dev.Sink); err != nil {
				return res, err
			}
			move(dev.Sink)
		} else {
			s.mu.Lock()
			s.jdspBroken = false
			s.mu.Unlock()
			res.ViaJamesDSP = true
		}
	} else {
		if err := srv.SetDefaultSink(dev.Sink); err != nil {
			return res, err
		}
		move(dev.Sink)
	}

	// The microphone follows (R7).
	if mic, name, ok := s.micFor(srv, cfg, dev, ev); ok {
		if err := srv.SetDefaultSource(mic); err != nil {
			ev.logf(LevelWarn, "setting the input to %s: %v", mic, err)
		} else {
			res.Mic, res.MicName = mic, name
			ev.logf(LevelInfo, "input follows: %s", name)
		}
	}

	// One notification per change of hardware, so the 5 s tick that lands
	// on the same device says nothing (R5.2).
	s.mu.Lock()
	changed := dev.Sink != s.lastPhysical
	s.lastPhysical = dev.Sink
	s.mu.Unlock()
	if changed && cfg.SwitchNotifications {
		input := "Unchanged"
		if res.MicName != "" {
			input = res.MicName
		}
		_, err := s.notifier().Send(notify.Notification{
			Title: "Audio Switched",
			Body:  "Output: " + dev.Name + "\nInput: " + input,
		})
		res.Notified = err == nil
	}
	return res, nil
}

// micFor is R7: the input that should follow this output, by the link the
// settings hold for its id.
func (s *Switcher) micFor(srv server, cfg config.Config, dev devices.Device, ev Events) (name, display string, ok bool) {
	link := cfg.MicLinks[dev.ID]
	if link == "" {
		link = config.MicAuto
	}
	if link == config.MicDefault {
		return "", "", false
	}
	sources, err := srv.Sources()
	if err != nil {
		ev.logf(LevelWarn, "listing the inputs: %v", err)
		return "", "", false
	}
	if link == config.MicAuto {
		src, found := Associate(dev.Properties, sources)
		if !found {
			return "", "", false
		}
		return src.Name, devices.DisplayName(src, nil), true
	}
	for _, src := range sources {
		if src.Name == link {
			return src.Name, devices.DisplayName(src, nil), true
		}
	}
	// A pinned input that is not present right now: set it anyway, as the
	// original did, so the server picks it up when it appears.
	return link, link, true
}

// associateKeys are the properties a sink and its own microphone share, in
// the order the original trusted them: the Bluetooth address is exact, the
// ALSA card index is the last resort.
var associateKeys = []string{
	"api.bluez5.address", "device.serial", "device.bus_path", "device.name", "alsa.card",
}

// Associate is R7.2: the first source sharing a property with the sink, by
// key in order of trust. No match means the input stays as it is.
func Associate(sinkProps map[string]string, sources []audio.Device) (audio.Device, bool) {
	for _, key := range associateKeys {
		want := sinkProps[key]
		if want == "" {
			continue
		}
		for _, src := range sources {
			if src.Properties[key] == want {
				return src, true
			}
		}
	}
	return audio.Device{}, false
}

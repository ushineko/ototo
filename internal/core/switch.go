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
	"github.com/ushineko/ototo/internal/loopback"
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
	SetVolume(sink string, percent int) (int, error)
	SetMute(sink string, mute bool) error
	Close() error
}

/*
dialAudio is how every operation reaches the sound server. A --server that
names a demo file gets the in-memory server over that file, one per
process, so the window's own code runs over invented devices for the
screenshot harness.
*/
func dialAudio(name string) (server, error) {
	if audio.IsDemo(name) {
		return demoServer(name)
	}
	return audio.Connect(name)
}

var (
	demoMu      sync.Mutex
	demoServers = map[string]*audio.Demo{}
)

func demoServer(name string) (*audio.Demo, error) {
	demoMu.Lock()
	defer demoMu.Unlock()
	if d, ok := demoServers[name]; ok {
		return d, nil
	}
	d, err := audio.LoadDemo(name)
	if err != nil {
		return nil, err
	}
	demoServers[name] = d
	return d, nil
}

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
	// Loopback is the line-in loopback (R9.3); nil uses the real commands.
	Loopback *loopback.Loopback
	// Player plays the switch sound into sink after lead of silence; nil
	// plays over the sound server.
	Player func(server, sink string, lead time.Duration, cfg config.Config) error

	dial func(server string) (server, error)

	mu           sync.Mutex
	jdspBroken   bool
	lastPhysical string
	// seen is when each sink was first listed; zero for the ones there at
	// the first listing. A sink first seen a moment ago is fresh (leadFor).
	seen map[string]time.Time
}

// noteSinks records the sinks listed now; the first listing is the machine
// as found, and nothing in it is fresh. A sink that is gone is forgotten,
// so headphones that reconnect are fresh again.
func (s *Switcher) noteSinks(sinks []audio.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := s.seen == nil
	if first {
		s.seen = map[string]time.Time{}
	}
	listed := map[string]bool{}
	for _, d := range sinks {
		listed[d.Name] = true
		if _, ok := s.seen[d.Name]; ok {
			continue
		}
		if first {
			s.seen[d.Name] = time.Time{}
		} else {
			s.seen[d.Name] = time.Now()
		}
	}
	for name := range s.seen {
		if !listed[name] {
			delete(s.seen, name)
		}
	}
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

func (s *Switcher) loopback() *loopback.Loopback {
	if s.Loopback == nil {
		s.Loopback = loopback.New()
	}
	return s.Loopback
}

// Close stops what the Switcher owns: a direct loopback child, if any.
func (s *Switcher) Close() {
	if s.Loopback != nil {
		s.Loopback.Close()
	}
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
			Kind: notify.KindFailure, Title: "Switch Failed", Body: err.Error(), Icon: notify.IconFailure,
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
	sinks, err := srv.Sinks()
	if err != nil {
		return SwitchResult{}, err
	}
	s.noteSinks(sinks)
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
	return s.switchTo(ctx, srv, cfg, dev, hasJamesDSP(sinks), req.Events)
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
	s.noteSinks(sinks)
	in.Sinks, in.DefaultSink = sinks, info.DefaultSink
	return devices.List(in), nil
}

// hasJamesDSP says the server has the JamesDSP sink. Without it there is
// nothing to route through, whatever the graph says: the graph is the
// machine's, and a demo server is not.
func hasJamesDSP(sinks []audio.Device) bool {
	for _, s := range sinks {
		if s.Name == devices.JamesDSPSink {
			return true
		}
	}
	return false
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
	_, _ = s.notifier().Send(notify.Notification{Kind: notify.KindConnecting, Title: "Connecting...", Body: "Connecting to " + dev.Name})
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

// switchTo is R5.1 and R5.2 for a device that has a sink. jdspSink says
// the server has the JamesDSP sink; without it the graph is not consulted.
func (s *Switcher) switchTo(ctx context.Context, srv server, cfg config.Config, dev devices.Device, jdspSink bool, ev Events) (SwitchResult, error) {
	res := SwitchResult{Device: dev}

	useJDSP := false
	if jdspSink && dev.Sink != devices.JamesDSPSink && !s.JamesDSPBroken() {
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
	// on the same device says nothing (R5.2). The sound follows the same
	// rule, and plays once the route is confirmed, into the device's own
	// sink, so it comes out of the new device and not out of a sink that
	// is still being rewired.
	s.mu.Lock()
	changed := dev.Sink != s.lastPhysical
	s.lastPhysical = dev.Sink
	s.mu.Unlock()
	if changed && cfg.SwitchSound {
		s.settleRoute(ctx, srv, dev.Sink, res.ViaJamesDSP, ev)
		server := req(srv)
		delay, lead := s.soundTiming(dev.Sink, cfg)
		go func() {
			time.Sleep(delay)
			started := time.Now()
			ev.logf(LevelDebug, "switch sound: into %s, %s after the switch, after %s of silence", dev.Sink, delay, lead)
			if err := s.play(server, dev.Sink, lead, cfg); err != nil {
				ev.logf(LevelWarn, "switch sound: %v", err)
				return
			}
			ev.logf(LevelDebug, "switch sound: drained after %s", time.Since(started).Round(time.Millisecond))
		}()
	}
	if changed && cfg.SwitchNotifications {
		input := "Unchanged"
		if res.MicName != "" {
			input = res.MicName
		}
		_, err := s.notifier().Send(notify.Notification{
			Kind:  notify.KindSwitched,
			Title: "Audio Switched",
			Body:  "Output: " + dev.Name + "\nInput: " + input,
		})
		res.Notified = err == nil
	}
	return res, nil
}

/*
The switch sound and the switch it follows. A stream opened the moment the
default sink changed lands in a sink that is still being rewired: through
JamesDSP the filter's output is relinked by pw-link, which the graph applies
after the command returns, and a clip played into the filter before the
link is up plays into nothing. So the sound is played into the device's own
sink, which needs no link, and only once the route is confirmed: the server
reports the default this switch set and, through JamesDSP, the graph
reports the filter linked to the device. The confirmation is polled every
routePoll for up to routeSettle; a route that never confirms plays anyway,
so a wrong reading costs a delay and not the sound.

A sink that was just resumed needs a moment before its first samples are
heard, and only a playing stream starts it, so the clip leads with silence
rather than waiting: soundLead for a sink that has been there.

A device that just appeared, first seen within freshFor, is another case:
headphones that just connected have a sink a second before their Bluetooth
transport is active, and PipeWire consumes a stream at rate while it is
pending, so the whole clip went by unheard; and then they play a chime of
their own, muting the stream under it. Measured on a WH-1000XM6: the
transport went active 1.3 s after the stream started, 0.9 s after a 350 ms
clip had drained, and the headphones rendered nothing for a few seconds
more, with nothing on the bus to mark the moment they did. So the
switch returns, and its sound is scheduled from the goroutine that plays
it for the settings' delay later, with freshLead of silence in front in
case the transport has gone idle again by then. A stream the server
refused is tried once more after soundRetry.
*/
const (
	routeSettle = 1500 * time.Millisecond
	routePoll   = 50 * time.Millisecond
	soundLead   = 100 * time.Millisecond
	freshLead   = 1500 * time.Millisecond
	freshFor    = 15 * time.Second
	soundRetry  = 300 * time.Millisecond
)

// soundTiming is when the switch sound for sink plays: the wait before the
// stream opens and the silence in front of the clip. A sink first seen
// within freshFor, or never listed before now, is fresh and waits the
// settings' delay.
func (s *Switcher) soundTiming(sink string, cfg config.Config) (delay, lead time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.seen[sink]
	if !ok {
		if s.seen == nil {
			s.seen = map[string]time.Time{}
		}
		t = time.Now()
		s.seen[sink] = t
	}
	if !t.IsZero() && time.Since(t) < freshFor {
		return time.Duration(cfg.SwitchSoundDelay) * time.Second, freshLead
	}
	return 0, soundLead
}

// settleRoute waits for the route to sink to be confirmed, as described
// above, and says whether it was.
func (s *Switcher) settleRoute(ctx context.Context, srv server, sink string, viaJDSP bool, ev Events) bool {
	want := sink
	if viaJDSP {
		want = devices.JamesDSPSink
	}
	deadline := time.Now().Add(routeSettle)
	for {
		ok := false
		if info, err := srv.Server(); err == nil && info.DefaultSink == want {
			ok = true
			if viaJDSP {
				target, err := s.graph().Target(ctx)
				ok = err == nil && target == sink
			}
		}
		if ok {
			return true
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			ev.logf(LevelWarn, "the route to %s did not confirm within %s; playing the switch sound anyway", sink, routeSettle)
			return false
		}
		select {
		case <-ctx.Done():
		case <-time.After(routePoll):
		}
	}
}

// play plays the switch sound through Player, or over the sound server,
// and once more after soundRetry when the first try failed.
func (s *Switcher) play(server, sink string, lead time.Duration, cfg config.Config) error {
	play := PlaySwitchSound
	if s.Player != nil {
		play = s.Player
	}
	if err := play(server, sink, lead, cfg); err == nil {
		return nil
	}
	time.Sleep(soundRetry)
	return play(server, sink, lead, cfg)
}

// PlaySwitchSound plays what the settings name, the WAV file or the chime,
// into sink, or the default output when sink is "", after lead of silence.
func PlaySwitchSound(server, sink string, lead time.Duration, cfg config.Config) error {
	sound := audio.Chime()
	if cfg.SwitchSoundFile != "" {
		var err error
		if sound, err = audio.ReadWAV(config.ExpandPath(cfg.SwitchSoundFile)); err != nil {
			return err
		}
	}
	return audio.Play(server, sink, sound.WithLead(lead))
}

// req names the server a connection was made to, for a sound played beside
// it; the fake server of a test names nothing.
func req(srv server) string {
	if n, ok := srv.(interface{ ServerName() string }); ok {
		return n.ServerName()
	}
	return ""
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

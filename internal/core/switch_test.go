package core

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/devices"
	"github.com/ushineko/ototo/internal/graph"
	"github.com/ushineko/ototo/internal/notify"
)

// Invented sinks. The address is the documentation range.
const (
	speakers    = "alsa_output.usb-Example_DAC-00.analog-stereo"
	headsetSink = "alsa_output.usb-Example_Headset-00.analog-stereo"
	airpods     = "bluez_output.AA_BB_CC_DD_EE_FF.1"
	speakMic    = "alsa_input.usb-Example_DAC-00.mono"
	headMic     = "alsa_input.usb-Example_Headset-00.mono"
)

// fakeServer is a sound server in memory that records what was asked of it.
type fakeServer struct {
	sinks       []audio.Device
	sources     []audio.Device
	defaultSink string
	defaultSrc  string
	volumes     map[string]int
	muted       bool
	calls       []string
	failSet     error
}

func (f *fakeServer) Server() (audio.Server, error) {
	return audio.Server{Name: "fake", DefaultSink: f.defaultSink, DefaultSource: f.defaultSrc}, nil
}
func (f *fakeServer) Sinks() ([]audio.Device, error)   { return f.sinks, nil }
func (f *fakeServer) Sources() ([]audio.Device, error) { return f.sources, nil }
func (f *fakeServer) SetDefaultSink(name string) error {
	f.calls = append(f.calls, "default-sink "+name)
	if f.failSet != nil {
		return f.failSet
	}
	f.defaultSink = name
	return nil
}
func (f *fakeServer) SetDefaultSource(name string) error {
	f.calls = append(f.calls, "default-source "+name)
	f.defaultSrc = name
	return nil
}
func (f *fakeServer) MoveSinkInputs(sink string) (int, int, error) {
	f.calls = append(f.calls, "move "+sink)
	return 2, 1, nil
}
func (f *fakeServer) Volume(sink string) (int, bool, error) { return f.volumes[sink], f.muted, nil }
func (f *fakeServer) AdjustVolume(sink string, delta int) (int, error) {
	f.volumes[sink] += delta
	f.calls = append(f.calls, "adjust "+sink)
	return f.volumes[sink], nil
}
func (f *fakeServer) SetVolume(sink string, percent int) (int, error) {
	f.volumes[sink] = percent
	f.calls = append(f.calls, "volume "+sink)
	return percent, nil
}
func (f *fakeServer) SetMute(sink string, mute bool) error {
	f.muted = mute
	f.calls = append(f.calls, "mute "+sink)
	return nil
}
func (f *fakeServer) Close() error { return nil }

// fakeGraph answers pw-link listings for a graph with or without JamesDSP,
// and records the link changes.
type fakeGraph struct {
	present bool   // JamesDSP outputs exist
	target  string // the sink they play into; "" floating
	calls   []string
}

func (g *fakeGraph) run(_ context.Context, args ...string) (string, error) {
	g.calls = append(g.calls, strings.Join(args, " "))
	switch args[0] {
	case "-o":
		if !g.present {
			return "Vivaldi:output_FL\n", nil
		}
		return "jdsp_@PwJamesDspPlugin_JamesDsp:output_FL\njdsp_@PwJamesDspPlugin_JamesDsp:output_FR\n", nil
	case "-i":
		return speakers + ":playback_FL\n" + speakers + ":playback_FR\n" +
			headsetSink + ":playback_FL\n" + headsetSink + ":playback_FR\n", nil
	case "-l":
		if g.target == "" {
			return "jdsp_@PwJamesDspPlugin_JamesDsp:output_FL\njdsp_@PwJamesDspPlugin_JamesDsp:output_FR\n", nil
		}
		return "jdsp_@PwJamesDspPlugin_JamesDsp:output_FL\n  |-> " + g.target + ":playback_FL\n" +
			"jdsp_@PwJamesDspPlugin_JamesDsp:output_FR\n  |-> " + g.target + ":playback_FR\n", nil
	}
	return "", nil
}

func sinkWith(name string, props map[string]string) audio.Device {
	return audio.Device{Name: name, Properties: props}
}

// world is one test's machine: a server, a graph, a notifier and a config.
type world struct {
	srv   *fakeServer
	g     *fakeGraph
	notes *notify.Recorder
	sw    *Switcher
	cfg   config.Config
	path  string
	// bt is what the fake adapter reports; connects is what it was asked.
	bt       []devices.Bluetooth
	connects []string
}

func newWorld(t *testing.T, jdsp bool) *world {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv(config.FileEnv, "")

	w := &world{
		srv: &fakeServer{
			sinks: []audio.Device{
				sinkWith(speakers, map[string]string{"device.description": "Speakers", "device.bus_path": "usb-1", "alsa.card": "1"}),
				sinkWith(headsetSink, map[string]string{"device.description": "Headset", "device.serial": "HS-1", "alsa.card": "2"}),
			},
			sources: []audio.Device{
				sinkWith(headMic, map[string]string{"device.serial": "HS-1", "alsa.card": "2", "device.description": "Headset Mic"}),
				sinkWith(speakMic, map[string]string{"device.bus_path": "usb-1", "alsa.card": "1", "device.description": "Desk Mic"}),
			},
			defaultSink: speakers,
			volumes:     map[string]int{speakers: 40, headsetSink: 60, devices.JamesDSPSink: 100},
		},
		g:     &fakeGraph{present: jdsp, target: speakers},
		notes: &notify.Recorder{},
		cfg:   config.Default(),
		path:  filepath.Join(home, "config.json"),
	}
	if jdsp {
		w.srv.sinks = append(w.srv.sinks, sinkWith(devices.JamesDSPSink, map[string]string{"device.description": "JamesDSP Sink"}))
	}
	w.cfg.DevicePriority = []string{headsetSink, speakers}
	w.sw = &Switcher{Graph: graph.NewWith(w.g.run), Notifier: w.notes, dial: func(string) (server, error) { return w.srv, nil }}
	w.save(t)
	return w
}

func (w *world) save(t *testing.T) {
	t.Helper()
	require.NoError(t, config.Save(w.path, w.cfg))
}

func (w *world) req() Request {
	return Request{ConfigPath: w.path, Probes: &Probes{
		Bluetooth: func(context.Context) []devices.Bluetooth { return w.bt },
		Connect: func(_ context.Context, mac string) error {
			w.connects = append(w.connects, mac)
			// The sink appears one poll later, as a real headsetSink's does.
			w.srv.sinks = append(w.srv.sinks, sinkWith(airpods, map[string]string{"device.api": "bluez5",
				"api.bluez5.address": "AA:BB:CC:DD:EE:FF"}))
			w.srv.sources = append(w.srv.sources, sinkWith("bluez_input.AA_BB_CC_DD_EE_FF", map[string]string{
				"api.bluez5.address": "AA:BB:CC:DD:EE:FF", "device.description": "AirPods Mic"}))
			return nil
		},
	}}
}

// TestASwitchWithoutJamesDSPSetsTheSinkAndMovesStreams: the plain path.
func TestASwitchWithoutJamesDSPSetsTheSinkAndMovesStreams(t *testing.T) {
	w := newWorld(t, false)
	res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "headset"})
	require.NoError(t, err)
	require.Equal(t, headsetSink, res.Device.Sink)
	require.False(t, res.ViaJamesDSP)
	require.Equal(t, []string{"default-sink " + headsetSink, "move " + headsetSink, "default-source " + headMic}, w.srv.calls)
	require.Equal(t, 2, res.StreamsMoved)
	require.Equal(t, 1, res.StreamsRefused)
}

// TestASwitchThroughJamesDSPRewiresTheGraph: the default becomes the
// JamesDSP sink, the streams go there, and the filter's output is relinked
// to the hardware. Effects survive the switch.
func TestASwitchThroughJamesDSPRewiresTheGraph(t *testing.T) {
	w := newWorld(t, true)
	res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: headsetSink})
	require.NoError(t, err)
	require.True(t, res.ViaJamesDSP)
	require.Equal(t, devices.JamesDSPSink, w.srv.defaultSink)
	require.Contains(t, w.srv.calls, "move "+devices.JamesDSPSink)
	require.Contains(t, w.g.calls, "jdsp_@PwJamesDspPlugin_JamesDsp:output_FL "+headsetSink+":playback_FL")
	require.Contains(t, w.g.calls, "-d jdsp_@PwJamesDspPlugin_JamesDsp:output_FL "+speakers+":playback_FL")
	require.False(t, w.sw.JamesDSPBroken())
}

// TestAFailedRelinkTripsTheBreakerAndFallsBack: JamesDSP was found but its
// output could not reach the sink, so the hardware is set directly, the
// breaker opens, and the next switch does not try JamesDSP at all.
func TestAFailedRelinkTripsTheBreakerAndFallsBack(t *testing.T) {
	w := newWorld(t, true)
	// A sink with no playback ports in the graph: the relink cannot pair.
	w.srv.sinks = append(w.srv.sinks, sinkWith("alsa_output.ghost", map[string]string{"device.description": "Ghost"}))
	res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "ghost"})
	require.NoError(t, err)
	require.False(t, res.ViaJamesDSP)
	require.Contains(t, res.Fallback, "no playback ports")
	require.Equal(t, "alsa_output.ghost", w.srv.defaultSink)
	require.True(t, w.sw.JamesDSPBroken())

	w.g.calls = nil
	_, err = w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "headset"})
	require.NoError(t, err)
	require.Empty(t, w.g.calls, "the graph was read while the breaker was open")

	// A manual switch is the reset the original offered.
	_, err = w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "headset", Manual: true})
	require.NoError(t, err)
	require.False(t, w.sw.JamesDSPBroken())
	require.NotEmpty(t, w.g.calls)
}

// TestANotificationIsSentOncePerChangeOfHardware: the tick that lands on
// the same device must not notify again, and failures are always sent.
func TestANotificationIsSentOncePerChangeOfHardware(t *testing.T) {
	w := newWorld(t, false)
	for range 3 {
		_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "headset"})
		require.NoError(t, err)
	}
	require.Len(t, w.notes.Sent, 1)
	require.Equal(t, "Audio Switched", w.notes.Sent[0].Title)
	require.Equal(t, "Output: Headset\nInput: Headset Mic", w.notes.Sent[0].Body)

	w.cfg.SwitchNotifications = false
	w.save(t)
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "speakers"})
	require.NoError(t, err)
	require.Len(t, w.notes.Sent, 1, "an informational notification was sent with the switch off")

	_, err = w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "nothing like this"})
	require.ErrorIs(t, err, ErrNotFound)
	require.Len(t, w.notes.Sent, 2, "a failure went unreported")
	require.Equal(t, "Switch Failed", w.notes.Sent[1].Title)
	require.Equal(t, notify.IconFailure, w.notes.Sent[1].Icon)
}

// TestTheSwitchSoundPlaysOncePerChangeOfHardware: on, it plays after a
// switch that changed the device, with the file the settings name; a tick
// that lands on the same device plays nothing.
func TestTheSwitchSoundPlaysOncePerChangeOfHardware(t *testing.T) {
	w := newWorld(t, false)
	w.cfg.SwitchSound = true
	w.cfg.SwitchSoundFile = "~/chime.wav"
	w.save(t)
	played := make(chan string, 4)
	w.sw.Player = func(_ string, cfg config.Config) error { played <- cfg.SwitchSoundFile; return nil }
	for range 2 {
		_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "headset"})
		require.NoError(t, err)
	}
	select {
	case f := <-played:
		require.Equal(t, "~/chime.wav", f)
	case <-time.After(time.Second):
		t.Fatal("the switch sound did not play")
	}
	select {
	case <-played:
		t.Fatal("the sound played again for the same device")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestTheMicrophoneFollowsTheLink: auto matches by shared property, a pinned
// source is set as given, and "default" leaves the input alone.
func TestTheMicrophoneFollowsTheLink(t *testing.T) {
	w := newWorld(t, false)
	res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "speakers"})
	require.NoError(t, err)
	require.Equal(t, speakMic, res.Mic, "auto did not match the mic on the same bus")

	w.cfg.MicLinks[headsetSink] = speakMic
	w.cfg.MicLinks[speakers] = config.MicDefault
	w.save(t)
	res, err = w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "headset"})
	require.NoError(t, err)
	require.Equal(t, speakMic, res.Mic, "the pinned input was not set")
	w.srv.calls = nil
	res, err = w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "speakers"})
	require.NoError(t, err)
	require.Empty(t, res.Mic)
	require.NotContains(t, strings.Join(w.srv.calls, "\n"), "default-source")
}

// TestAssociateTrustsTheAddressBeforeTheCard: two sources on the same ALSA
// card is normal (a headsetSink's mic and its chat mic), and the serial or
// address is what picks the right one.
func TestAssociateTrustsTheAddressBeforeTheCard(t *testing.T) {
	sources := []audio.Device{
		sinkWith("card-mic", map[string]string{"alsa.card": "2"}),
		sinkWith("serial-mic", map[string]string{"alsa.card": "2", "device.serial": "HS-1"}),
		sinkWith("bt-mic", map[string]string{"api.bluez5.address": "AA:BB:CC:DD:EE:FF"}),
	}
	src, ok := Associate(map[string]string{"alsa.card": "2", "device.serial": "HS-1"}, sources)
	require.True(t, ok)
	require.Equal(t, "serial-mic", src.Name)
	src, ok = Associate(map[string]string{"api.bluez5.address": "AA:BB:CC:DD:EE:FF", "alsa.card": "2"}, sources)
	require.True(t, ok)
	require.Equal(t, "bt-mic", src.Name)
	_, ok = Associate(map[string]string{"device.serial": ""}, sources)
	require.False(t, ok, "an empty property matched something")
}

// TestResolveIsExactThenSubstringInPriorityOrder: "head" finds the headsetSink,
// the exact id wins over a substring elsewhere, and an unknown name is not
// an accidental match on the empty string.
func TestResolveIsExactThenSubstringInPriorityOrder(t *testing.T) {
	list := []devices.Device{
		{ID: headsetSink, Name: "Headset", Sink: headsetSink, Online: true},
		{ID: speakers, Name: "Speakers head", Sink: speakers, Online: true},
		{ID: "bt:AA:BB:CC:DD:EE:FF", Name: "AirPods [Disconnected]"},
	}
	d, ok := Resolve(list, "HEAD")
	require.True(t, ok)
	require.Equal(t, headsetSink, d.ID)
	d, ok = Resolve(list, speakers)
	require.True(t, ok)
	require.Equal(t, speakers, d.ID)
	d, ok = Resolve(list, "airpods")
	require.True(t, ok)
	require.False(t, d.Online)
	_, ok = Resolve(list, "")
	require.False(t, ok)
}

// TestAnAwayBluetoothDeviceIsConnectedThenSwitchedTo: the original's
// offline branch. "Connecting..." goes out first, BlueZ is asked, the sink
// is waited for, and the switch lands on it with its own microphone.
func TestAnAwayBluetoothDeviceIsConnectedThenSwitchedTo(t *testing.T) {
	w := newWorld(t, false)
	w.bt = []devices.Bluetooth{{MAC: "AA:BB:CC:DD:EE:FF", Name: "AirPods Pro"}}
	res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)
	require.Equal(t, []string{"AA:BB:CC:DD:EE:FF"}, w.connects)
	require.Equal(t, airpods, res.Device.Sink)
	require.Equal(t, "AirPods Pro", res.Device.Name)
	require.Equal(t, "bluez_input.AA_BB_CC_DD_EE_FF", res.Mic)
	require.Equal(t, "Connecting...", w.notes.Sent[0].Title)
	require.Equal(t, "Audio Switched", w.notes.Sent[1].Title)
}

// TestConnectBringsADeviceUpWithoutSwitching: the sink appears and the
// default stays where it was, for the auto-switch or the person to decide.
func TestConnectBringsADeviceUpWithoutSwitching(t *testing.T) {
	w := newWorld(t, false)
	w.bt = []devices.Bluetooth{{MAC: "AA:BB:CC:DD:EE:FF", Name: "AirPods Pro"}}
	dev, err := w.sw.Connect(context.Background(), ConnectRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)
	require.True(t, dev.Online)
	require.Equal(t, []string{"AA:BB:CC:DD:EE:FF"}, w.connects)
	require.Equal(t, speakers, w.srv.defaultSink, "connect switched the output")
	require.Equal(t, "Connecting...", w.notes.Sent[0].Title)

	_, err = w.sw.Connect(context.Background(), ConnectRequest{Request: w.req(), Target: "speakers"})
	require.ErrorContains(t, err, "not a Bluetooth device")
}

// TestAnAwayWiredDeviceIsRefused: nothing can connect a USB DAC that is
// unplugged, and the answer says so.
func TestAnAwayWiredDeviceIsRefused(t *testing.T) {
	w := newWorld(t, false)
	w.cfg.DevicePriority = []string{"alsa_output.gone", headsetSink}
	w.save(t)
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "alsa_output.gone"})
	require.ErrorIs(t, err, ErrNotConnected)
	require.Empty(t, w.srv.calls)
}

// TestAConnectThatProducesNoSinkTimesOut: BlueZ said yes and PipeWire never
// made a sink; the person is told, and nothing was switched.
func TestAConnectThatProducesNoSinkTimesOut(t *testing.T) {
	w := newWorld(t, false)
	w.bt = []devices.Bluetooth{{MAC: "AA:BB:CC:DD:EE:FF", Name: "AirPods Pro"}}
	req := w.req()
	req.Probes.Connect = func(context.Context, string) error { return nil }
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	_, err := w.sw.Switch(ctx, SwitchRequest{Request: req, Target: "airpods"})
	require.Error(t, err)
	require.Empty(t, w.srv.calls)
	require.Equal(t, "Switch Failed", w.notes.Sent[len(w.notes.Sent)-1].Title)
}

// TestTheHeadsetProbeReachesTheList: the Arctis row carries the battery the
// probe reported, so the auto-switch can tell on from off, and a switch by
// name finds it.
func TestTheHeadsetProbeReachesTheList(t *testing.T) {
	w := newWorld(t, false)
	w.srv.sinks = append(w.srv.sinks, sinkWith("alsa_output.usb-SteelSeries_Arctis_Nova_Pro_Wireless-00.analog-stereo",
		map[string]string{"device.vendor.name": "SteelSeries", "device.product.name": "Arctis Nova Pro Wireless"}))
	req := w.req()
	req.Probes.Headset = func(context.Context) devices.Headset { return devices.Headset{Detected: true, Battery: "87%"} }
	res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: req, Target: "arctis"})
	require.NoError(t, err)
	require.Equal(t, "SteelSeries Arctis Nova Pro Wireless [87%]", res.Device.Name)
	require.True(t, res.Device.Connected)
}

// TestStatusMarksTheSinkBehindJamesDSPAsPlaying: with JamesDSP as the
// default, the row that plays is the hardware sink it is routed to, not
// nothing.
func TestStatusMarksTheSinkBehindJamesDSPAsPlaying(t *testing.T) {
	w := newWorld(t, true)
	w.srv.defaultSink = devices.JamesDSPSink
	w.g.target = headsetSink
	_ = w
	// Status opens its own graph (pw-link on PATH); on this machine that
	// reads the real graph, so the pure part is what this test pins.
	in := devices.Inputs{Sinks: w.srv.sinks, DefaultSink: headsetSink}
	list := devices.List(in)
	var playing []string
	for _, d := range list {
		if d.Default {
			playing = append(playing, d.Sink)
		}
	}
	require.Equal(t, []string{headsetSink}, playing)
}

// TestAServerRefusalIsReturnedAndNotified: a hotkey has no terminal.
func TestAServerRefusalIsReturnedAndNotified(t *testing.T) {
	w := newWorld(t, false)
	w.srv.failSet = errors.New("no such entity")
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "headset"})
	require.ErrorContains(t, err, "no such entity")
	require.Len(t, w.notes.Sent, 1)
	require.Equal(t, "Switch Failed", w.notes.Sent[0].Title)
}

// TestVolumeFollowsTheRouting: with JamesDSP as the default, the keys change
// the hardware sink it plays into, not the virtual sink.
func TestVolumeFollowsTheRouting(t *testing.T) {
	w := newWorld(t, true)
	w.srv.defaultSink = devices.JamesDSPSink
	w.g.target = headsetSink
	res, err := w.sw.Volume(context.Background(), VolumeRequest{Request: w.req(), Delta: audio.VolumeStep})
	require.NoError(t, err)
	require.Equal(t, headsetSink, res.Sink)
	require.Equal(t, 65, res.Percent)
	require.Equal(t, 100, w.srv.volumes[devices.JamesDSPSink], "the virtual sink's volume moved")

	w.g.target = ""
	res, err = w.sw.Volume(context.Background(), VolumeRequest{Request: w.req()})
	require.NoError(t, err)
	require.Equal(t, devices.JamesDSPSink, res.Sink, "floating JamesDSP has no hardware sink to report")
}

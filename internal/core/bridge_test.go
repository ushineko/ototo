package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/bridge"
	"github.com/ushineko/ototo/internal/devices"
	"github.com/ushineko/ototo/internal/graph"
)

const (
	jdspFL = "jdsp_@PwJamesDspPlugin_JamesDsp:output_FL"
	jdspFR = "jdsp_@PwJamesDspPlugin_JamesDsp:output_FR"
	brFL   = graph.BridgeOut + ":output_FL"
	brFR   = graph.BridgeOut + ":output_FR"
)

/*
bridgeGraph is a PipeWire graph in memory with JamesDSP, the bridge once its
child starts, the wired sinks and a Bluetooth headset whose ports follow its
profile: FL/FR in A2DP, one MONO port in handsfree. Links are held as a set,
so a listing shows what the calls made.
*/
type bridgeGraph struct {
	bridgeUp bool
	mono     bool
	links    map[[2]string]bool
	changes  []string
	// onLink runs after each link is made, so a test can have the headset
	// start when JamesDSP plays into it.
	onLink func(out, in string)
}

func newBridgeGraph() *bridgeGraph {
	return &bridgeGraph{links: map[[2]string]bool{
		{jdspFL, speakers + ":playback_FL"}: true,
		{jdspFR, speakers + ":playback_FR"}: true,
	}}
}

func (g *bridgeGraph) headsetPorts() []string {
	if g.mono {
		return []string{airpods + ":playback_MONO"}
	}
	return []string{airpods + ":playback_FL", airpods + ":playback_FR"}
}

// profile switches the headset, which destroys its node and its links.
func (g *bridgeGraph) profile(mono bool) {
	g.mono = mono
	for l := range g.links {
		if strings.HasPrefix(l[1], airpods+":") {
			delete(g.links, l)
		}
	}
}

func (g *bridgeGraph) run(_ context.Context, args ...string) (string, error) {
	switch strings.Join(args, " ") {
	case "-o":
		out := jdspFL + "\n" + jdspFR + "\n"
		if g.bridgeUp {
			out += brFL + "\n" + brFR + "\n"
		}
		return out, nil
	case "-i":
		in := speakers + ":playback_FL\n" + speakers + ":playback_FR\n" +
			headsetSink + ":playback_FL\n" + headsetSink + ":playback_FR\n" +
			strings.Join(g.headsetPorts(), "\n") + "\n"
		if g.bridgeUp {
			in += graph.BridgeSink + ":playback_FL\n" + graph.BridgeSink + ":playback_FR\n"
		}
		return in, nil
	case "-l":
		var b strings.Builder
		for _, out := range []string{jdspFL, jdspFR, brFL, brFR} {
			b.WriteString(out + "\n")
			for l := range g.links {
				if l[0] == out {
					b.WriteString("  |-> " + l[1] + "\n")
				}
			}
		}
		return b.String(), nil
	}
	g.changes = append(g.changes, strings.Join(args, " "))
	if args[0] == "-d" {
		delete(g.links, [2]string{args[1], args[2]})
	} else {
		g.links[[2]string{args[0], args[1]}] = true
		if g.onLink != nil {
			g.onLink(args[0], args[1])
		}
	}
	return "", nil
}

func (g *bridgeGraph) linked(out, in string) bool { return g.links[[2]string{out, in}] }

// dropBridge is the bridge child exiting: PipeWire removes its nodes and
// every link to and from them.
func (g *bridgeGraph) dropBridge() {
	for l := range g.links {
		if strings.HasPrefix(l[0], graph.BridgeOut+":") || strings.HasPrefix(l[1], graph.BridgeSink+":") {
			delete(g.links, l)
		}
	}
}

// bridgeWorld is a world with JamesDSP, a connected Bluetooth headset and a
// Switcher that owns a bridge whose child is a stand-in.
func bridgeWorld(t *testing.T, startErr error) (*world, *bridgeGraph, *int) {
	t.Helper()
	w := newWorld(t, true)
	w.srv.defaultSink = devices.JamesDSPSink
	w.srv.sinks = append(w.srv.sinks, sinkWith(airpods, map[string]string{
		"device.api": "bluez5", "api.bluez5.address": "AA:BB:CC:DD:EE:FF", "bluez.alias": "AirPods Pro",
	}))
	w.bt = []devices.Bluetooth{{MAC: "AA:BB:CC:DD:EE:FF", Name: "AirPods Pro", Connected: true}}
	g := newBridgeGraph()
	starts := 0
	alive := false
	start := func(string, ...string) (func(), func() bool, error) {
		if startErr != nil {
			return nil, nil, startErr
		}
		starts++
		alive, g.bridgeUp = true, true
		return func() { alive, g.bridgeUp = false, false; g.dropBridge() }, func() bool { return alive }, nil
	}
	w.sw.Graph = graph.NewWith(g.run)
	w.sw.Bridge = bridge.NewWith(start)
	return w, g, &starts
}

// TestJamesDSPReachesABluetoothHeadsetThroughTheBridge: JamesDSP plays into
// the stereo bridge and the bridge into the headset, never JamesDSP into the
// headset directly (spec 006 R2).
func TestJamesDSPReachesABluetoothHeadsetThroughTheBridge(t *testing.T) {
	w, g, starts := bridgeWorld(t, nil)
	res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)
	require.True(t, res.ViaJamesDSP)
	require.Equal(t, 1, *starts)
	require.True(t, g.linked(jdspFL, graph.BridgeSink+":playback_FL"))
	require.True(t, g.linked(jdspFR, graph.BridgeSink+":playback_FR"))
	require.True(t, g.linked(brFL, airpods+":playback_FL"))
	require.True(t, g.linked(brFR, airpods+":playback_FR"))
	require.False(t, g.linked(jdspFL, airpods+":playback_FL"))
	require.False(t, g.linked(jdspFL, speakers+":playback_FL"), "the old route is gone")
	require.Contains(t, w.srv.calls, "level ototo_bt_bridge_out 1.00", "stereo is at unity")
}

// TestAProfileSwitchIsRewiredToTheMonoPort: handsfree recreates the headset
// with one MONO port; the maintenance on the next event links both bridge
// channels to it, and a second pass changes nothing (R3).
func TestAProfileSwitchIsRewiredToTheMonoPort(t *testing.T) {
	w, g, _ := bridgeWorld(t, nil)
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)

	g.profile(true)
	w.sw.MaintainBridge(context.Background(), w.req())
	require.True(t, g.linked(brFL, airpods+":playback_MONO"))
	require.True(t, g.linked(brFR, airpods+":playback_MONO"))
	require.True(t, g.linked(jdspFL, graph.BridgeSink+":playback_FL"), "JamesDSP stays on the bridge")
	require.Equal(t, "level ototo_bt_bridge_out 0.50", w.srv.calls[len(w.srv.calls)-1],
		"two channels summed into one port are halved, or they clip")

	before := len(g.changes)
	w.sw.MaintainBridge(context.Background(), w.req())
	require.Len(t, g.changes, before, "correct links are left alone")

	g.profile(false)
	w.sw.MaintainBridge(context.Background(), w.req())
	require.True(t, g.linked(brFL, airpods+":playback_FL"))
	require.Equal(t, "level ototo_bt_bridge_out 1.00", w.srv.calls[len(w.srv.calls)-1], "back in A2DP at unity")
}

// TestAWiredDeviceDoesNotUseTheBridge: non-Bluetooth routing is as before
// (R2.2), and the bridge child is not started for it.
func TestAWiredDeviceDoesNotUseTheBridge(t *testing.T) {
	w, g, starts := bridgeWorld(t, nil)
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "headset"})
	require.NoError(t, err)
	require.Equal(t, 0, *starts)
	require.True(t, g.linked(jdspFL, headsetSink+":playback_FL"))
	require.True(t, g.linked(jdspFR, headsetSink+":playback_FR"))
}

// TestNoBridgeFallsBackToTheDirectRoute: with pw-loopback missing, the
// switch still lands, JamesDSP is relinked directly as before, and the
// failure is logged once across switches (R1.4).
func TestNoBridgeFallsBackToTheDirectRoute(t *testing.T) {
	w, g, _ := bridgeWorld(t, errors.New("exec: \"pw-loopback\": not found"))
	var warns []string
	req := w.req()
	req.Events = Events{Log: func(level Level, msg string) {
		if level == LevelWarn && strings.Contains(msg, "bridge") {
			warns = append(warns, msg)
		}
	}}
	for range 2 {
		res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: req, Target: "airpods"})
		require.NoError(t, err)
		require.True(t, res.ViaJamesDSP)
	}
	require.True(t, g.linked(jdspFL, airpods+":playback_FL"))
	require.Len(t, warns, 1)
	require.True(t, w.sw.BridgeFailed())
}

// TestTheVolumeIsTheHeadsetsThroughTheBridge: the keys and the window act on
// the headset, not on the bridge (R6).
func TestTheVolumeIsTheHeadsetsThroughTheBridge(t *testing.T) {
	w, _, _ := bridgeWorld(t, nil)
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)
	sink, err := w.sw.playingSink(context.Background(), w.srv)
	require.NoError(t, err)
	require.Equal(t, airpods, sink)
}

// TestDecideInsistsOnTheBridgeForBluetooth: JamesDSP straight into the
// headset is the silent handsfree route, so the tick re-routes it; through
// the bridge into the wanted headset, the tick leaves it (R5).
func TestDecideInsistsOnTheBridgeForBluetooth(t *testing.T) {
	ap := dev("bt:AA:BB:CC:DD:EE:FF", airpods, true, true)
	sp := dev(speakers, speakers, true, true)
	base := snapshot{list: []devices.Device{ap, sp}, defaultSink: devices.JamesDSPSink, currentValid: true,
		jdspSink: true, jdspOutputs: true, jdspTarget: airpods, bridgeWanted: true}

	direct := base
	d := decide(direct)
	require.True(t, d.switchNow, d.reason)

	bridged := base
	bridged.jdspBridged = true
	d = decide(bridged)
	require.False(t, d.switchNow, d.reason)

	failed := base
	failed.bridgeWanted = false
	d = decide(failed)
	require.False(t, d.switchNow, "without a usable bridge the direct route stands: "+d.reason)
}

// btSink is the fake server's Bluetooth sink, to set its node and state.
func btSink(w *world) *audio.Device {
	for i := range w.srv.sinks {
		if w.srv.sinks[i].Name == airpods {
			return &w.srv.sinks[i]
		}
	}
	panic("no headset in the world")
}

// a2dpWorld is bridgeWorld with the headset's card in A2DP, asleep, and its
// transport idle, as a headset is after ototo restarts.
func a2dpWorld(t *testing.T) (*world, *bridgeGraph, *int) {
	t.Helper()
	w, g, starts := bridgeWorld(t, nil)
	w.srv.btProfile = "a2dp-sink-sbc_xq"
	btSink(w).State = audio.StateSuspended
	transport := "idle"
	w.transport = func() string { return transport }
	return w, g, starts
}

// setTransport says what BlueZ reports from now on.
func setTransport(w *world, state string) { w.transport = func() string { return state } }

/*
startsOnJamesDSP is the headset as measured: JamesDSP linked into it starts
the node at once, and BlueZ streams to it a few reads of the transport
later (some tens of milliseconds on the development host).
*/
func startsOnJamesDSP(w *world, g *bridgeGraph) {
	g.onLink = func(out, in string) {
		if !strings.HasPrefix(out, "jdsp_") || !strings.HasPrefix(in, airpods+":") {
			return
		}
		btSink(w).State = audio.StateRunning
		reads := 0
		w.transport = func() string {
			if reads++; reads <= 3 {
				return "pending"
			}
			return "active"
		}
	}
}

// changeIndex is the position of the first graph change that starts with
// prefix, or -1.
func changeIndex(g *bridgeGraph, prefix string) int {
	for i, c := range g.changes {
		if strings.HasPrefix(c, prefix) {
			return i
		}
	}
	return -1
}

// shortWake keeps a test of a headset that never streams from waiting 2 s.
func shortWake(t *testing.T) {
	t.Helper()
	timeout, poll := wakeTimeout, wakePoll
	wakeTimeout, wakePoll = 20*time.Millisecond, time.Millisecond
	t.Cleanup(func() { wakeTimeout, wakePoll = timeout, poll })
}

// TestAnAsleepHeadsetIsWokenBeforeTheBridgeTakesIt (spec 007 R2.2, R2.3):
// 0.1.7 wired a new bridge to an asleep headset, which then played nothing
// with every reading healthy. JamesDSP now wakes the headset itself, and
// the bridge is wired to it only once BlueZ streams to it.
func TestAnAsleepHeadsetIsWokenBeforeTheBridgeTakesIt(t *testing.T) {
	w, g, _ := a2dpWorld(t)
	startsOnJamesDSP(w, g)

	res, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)
	require.True(t, res.ViaJamesDSP)

	direct := changeIndex(g, jdspFL+" "+airpods+":playback_FL")
	wired := changeIndex(g, brFL+" "+airpods+":playback_FL")
	onto := changeIndex(g, jdspFL+" "+graph.BridgeSink+":playback_FL")
	require.GreaterOrEqual(t, direct, 0, "JamesDSP never woke the headset: %v", g.changes)
	require.Greater(t, wired, direct, "the bridge was wired to the headset before it ran: %v", g.changes)
	require.Greater(t, onto, wired, "JamesDSP moved onto a bridge that went nowhere: %v", g.changes)

	require.True(t, g.linked(jdspFL, graph.BridgeSink+":playback_FL"))
	require.True(t, g.linked(brFL, airpods+":playback_FL"))
	require.False(t, g.linked(jdspFL, airpods+":playback_FL"), "the direct link was left beside the bridge")
}

// TestARunningHeadsetWaitsForBlueZ (spec 007 R2.2, R2.4, D5): PipeWire
// reports the headset running some tens of milliseconds before BlueZ
// streams to it, and a handover in that window was silent 3/3 on the
// development host. Running is not enough; the direct route stands, and the
// tick does not fight it, until the transport is active.
func TestARunningHeadsetWaitsForBlueZ(t *testing.T) {
	shortWake(t)
	w, g, _ := a2dpWorld(t)

	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)
	require.True(t, g.linked(jdspFL, airpods+":playback_FL"), "JamesDSP is not waking the headset")
	require.False(t, g.linked(brFL, airpods+":playback_FL"), "the bridge was wired to a headset that never ran")

	btSink(w).State = audio.StateRunning
	setTransport(w, "pending")
	before := len(g.changes)
	w.sw.MaintainBridge(context.Background(), w.req())
	require.Len(t, g.changes, before, "the bridge took a headset BlueZ is not streaming to yet")

	setTransport(w, "active")
	w.sw.MaintainBridge(context.Background(), w.req())
	require.True(t, g.linked(jdspFL, graph.BridgeSink+":playback_FL"))
	require.True(t, g.linked(brFL, airpods+":playback_FL"))
	require.False(t, g.linked(jdspFL, airpods+":playback_FL"))
}

// TestAReconnectedHeadsetIsWokenAgain (spec 007 D2): a reconnect recreates
// the headset's node, which the bridge has never held, so it is woken
// directly again before the bridge takes it. Once held, the node suspending
// and resuming through the bridge is left alone, which played 2/2.
func TestAReconnectedHeadsetIsWokenAgain(t *testing.T) {
	w, g, _ := a2dpWorld(t)
	btSink(w).Index, btSink(w).State = 7, audio.StateRunning
	setTransport(w, "active")
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)
	require.True(t, g.linked(brFL, airpods+":playback_FL"), "a streaming headset is wired straight away, as in 0.1.7")

	btSink(w).State = audio.StateSuspended
	setTransport(w, "idle")
	before := len(g.changes)
	w.sw.MaintainBridge(context.Background(), w.req())
	require.Len(t, g.changes, before, "a node the bridge holds was woken again when it suspended")

	g.profile(false) // the reconnect drops the headset's links
	btSink(w).Index = 8
	w.sw.MaintainBridge(context.Background(), w.req())
	require.True(t, g.linked(jdspFL, airpods+":playback_FL"), "the new node was not woken directly")
	require.False(t, g.linked(brFL, airpods+":playback_FL"), "the bridge was wired to the new node asleep")

	btSink(w).State = audio.StateRunning
	setTransport(w, "active")
	w.sw.MaintainBridge(context.Background(), w.req())
	require.True(t, g.linked(brFL, airpods+":playback_FL"))
	require.True(t, g.linked(jdspFL, graph.BridgeSink+":playback_FL"))
	require.False(t, g.linked(jdspFL, airpods+":playback_FL"))
}

// TestHandsfreeIsWiredAtOnce (spec 007 D6): handsfree has no A2DP transport
// to wait for, and JamesDSP straight into its mono port is the silence of
// #48. The new handsfree node gets the bridge at once, halved, as spec 006
// wired it and its test passed by ear.
func TestHandsfreeIsWiredAtOnce(t *testing.T) {
	w, g, _ := a2dpWorld(t)
	btSink(w).Index, btSink(w).State = 7, audio.StateRunning
	setTransport(w, "active")
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)

	w.srv.btProfile = "headset-head-unit"
	g.profile(true)
	btSink(w).Index, btSink(w).State = 8, audio.StateSuspended
	setTransport(w, "")
	w.sw.MaintainBridge(context.Background(), w.req())
	require.True(t, g.linked(brFL, airpods+":playback_MONO"))
	require.True(t, g.linked(brFR, airpods+":playback_MONO"))
	require.True(t, g.linked(jdspFL, graph.BridgeSink+":playback_FL"), "JamesDSP left the bridge for the mono port")
	require.False(t, g.linked(jdspFL, airpods+":playback_MONO"))
	require.Equal(t, "level ototo_bt_bridge_out 0.50", w.srv.calls[len(w.srv.calls)-1])
}

// TestARestartedBridgeHoldsNothing (spec 007 D2): the 0.1.7 trigger was a
// restart, a new bridge child wired to the asleep headset it had held
// before. A new child holds no node, even the same one.
func TestARestartedBridgeHoldsNothing(t *testing.T) {
	shortWake(t)
	w, g, starts := a2dpWorld(t)
	btSink(w).State = audio.StateRunning
	setTransport(w, "active")
	_, err := w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)

	w.sw.Bridge.Close()
	btSink(w).State = audio.StateSuspended
	setTransport(w, "idle")
	_, err = w.sw.Switch(context.Background(), SwitchRequest{Request: w.req(), Target: "airpods"})
	require.NoError(t, err)
	require.Equal(t, 2, *starts)
	require.True(t, g.linked(jdspFL, airpods+":playback_FL"), "a new bridge was taken for one that holds the headset")
	require.False(t, g.linked(brFL, airpods+":playback_FL"))
}

// TestDecideLetsAWakingHeadsetBe (spec 007 R3): JamesDSP straight into the
// headset the bridge is waiting for is the wake, not a reason to switch
// again; into a headset the bridge does not serve, it still is.
func TestDecideLetsAWakingHeadsetBe(t *testing.T) {
	ap := dev("bt:AA:BB:CC:DD:EE:FF", airpods, true, true)
	sp := dev(speakers, speakers, true, true)
	waking := snapshot{list: []devices.Device{ap, sp}, defaultSink: devices.JamesDSPSink, currentValid: true,
		jdspSink: true, jdspOutputs: true, jdspTarget: airpods, bridgeWanted: true, bridgeServes: airpods}
	d := decide(waking)
	require.False(t, d.switchNow, d.reason)

	elsewhere := waking
	elsewhere.bridgeServes = ""
	d = decide(elsewhere)
	require.True(t, d.switchNow, d.reason)
}

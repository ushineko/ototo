package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
	}
	return "", nil
}

func (g *bridgeGraph) linked(out, in string) bool { return g.links[[2]string{out, in}] }

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
		return func() { alive, g.bridgeUp = false, false }, func() bool { return alive }, nil
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

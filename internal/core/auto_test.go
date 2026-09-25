package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/devices"
)

func dev(id, sink string, online, connected bool) devices.Device {
	return devices.Device{ID: id, Name: id, Sink: sink, Online: online, Connected: connected}
}

// TestDecideFollowsTheOriginalsBranches pins run_auto_switch, case by case.
func TestDecideFollowsTheOriginalsBranches(t *testing.T) {
	hs := dev(headset, headset, true, true)
	sp := dev(speakers, speakers, true, true)
	hsOff := dev(headset, headset, true, false)
	ap := dev("bt:AA:BB:CC:DD:EE:FF", "", false, false)

	cases := []struct {
		name      string
		sn        snapshot
		wantSink  string
		switchNow bool
		reset     bool
	}{
		{"the first available device is playing: nothing to do",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: headset, currentValid: true}, headset, false, false},
		{"the default is not the first available device: switch",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: speakers, currentValid: true}, headset, true, false},
		{"an away device is skipped for the next one",
			snapshot{list: []devices.Device{ap, hs, sp}, defaultSink: speakers, currentValid: true}, headset, true, false},
		{"a disconnected device is skipped for the next one",
			snapshot{list: []devices.Device{hsOff, sp}, defaultSink: speakers, currentValid: true}, speakers, false, false},
		{"JamesDSP present and bypassed: enforce the routing",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: headset, currentValid: true, jdspSink: true, jdspOutputs: true}, headset, true, false},
		{"JamesDSP present but its filter is not running: leave the hardware",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: headset, currentValid: true, jdspSink: true}, headset, false, false},
		{"the breaker is open and the outputs are back: close it and enforce",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: headset, currentValid: true, jdspSink: true, jdspOutputs: true, jdspBroken: true}, headset, true, true},
		{"the breaker is open and the outputs are still gone: leave it",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: headset, currentValid: true, jdspSink: true, jdspBroken: true}, headset, false, false},
		{"JamesDSP plays into the first available device: correct",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: devices.JamesDSPSink, currentValid: true, jdspSink: true, jdspOutputs: true, jdspTarget: headset}, headset, false, false},
		{"JamesDSP plays into the wrong device: switch",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: devices.JamesDSPSink, currentValid: true, jdspSink: true, jdspOutputs: true, jdspTarget: speakers}, headset, true, false},
		{"JamesDSP is floating: switch",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: devices.JamesDSPSink, currentValid: true, jdspSink: true, jdspOutputs: true}, headset, true, false},
		{"JamesDSP is idle with no outputs: leave it",
			snapshot{list: []devices.Device{hs, sp}, defaultSink: devices.JamesDSPSink, currentValid: true, jdspSink: true}, headset, false, false},
		{"nothing in the order is available and the default cannot play: fallback",
			snapshot{list: []devices.Device{hsOff, dev("alsa_output.other", "alsa_output.other", true, true)}, defaultSink: headset, currentValid: false}, "alsa_output.other", true, false},
		{"nothing in the order is available but the default plays: leave it",
			snapshot{list: []devices.Device{hsOff}, defaultSink: speakers, currentValid: true}, "", false, false},
		{"no device can play at all",
			snapshot{list: []devices.Device{hsOff, ap}, defaultSink: headset, currentValid: false}, "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := decide(c.sn)
			if c.wantSink == "" {
				require.Nil(t, d.target)
			} else {
				require.NotNil(t, d.target)
				require.Equal(t, c.wantSink, d.target.Sink)
			}
			require.Equal(t, c.switchNow, d.switchNow, d.reason)
			require.Equal(t, c.reset, d.resetBreaker)
			require.NotEmpty(t, d.reason)
		})
	}
}

// TestAutoSwitchDoesNothingWhenOff: the tick runs on a timer, and off means
// off, without a server connection.
func TestAutoSwitchDoesNothingWhenOff(t *testing.T) {
	w := newWorld(t, false)
	res, err := w.sw.AutoSwitch(context.Background(), AutoSwitchRequest{Request: w.req()})
	require.NoError(t, err)
	require.False(t, res.Enabled)
	require.Empty(t, w.srv.calls)
}

// TestAutoSwitchMovesToTheFirstAvailableDeviceOnce: the first tick switches
// to the headset, the second finds it playing and does nothing.
func TestAutoSwitchMovesToTheFirstAvailableDeviceOnce(t *testing.T) {
	w := newWorld(t, false)
	w.cfg.AutoSwitch = true
	w.save(t)
	res, err := w.sw.AutoSwitch(context.Background(), AutoSwitchRequest{Request: w.req()})
	require.NoError(t, err)
	require.True(t, res.Switched)
	require.Equal(t, headset, w.srv.defaultSink)
	require.Len(t, w.notes.Sent, 1)

	res, err = w.sw.AutoSwitch(context.Background(), AutoSwitchRequest{Request: w.req()})
	require.NoError(t, err)
	require.False(t, res.Switched)
	require.Len(t, w.notes.Sent, 1)
}

// TestAutoSwitchClosesTheBreakerWhenOutputsReturn: R5.4's third reset.
func TestAutoSwitchClosesTheBreakerWhenOutputsReturn(t *testing.T) {
	w := newWorld(t, true)
	w.cfg.AutoSwitch = true
	w.save(t)
	w.sw.jdspBroken = true
	w.srv.defaultSink = headset
	res, err := w.sw.AutoSwitch(context.Background(), AutoSwitchRequest{Request: w.req()})
	require.NoError(t, err)
	require.False(t, w.sw.JamesDSPBroken())
	require.True(t, res.Switched)
	require.True(t, res.Switch.ViaJamesDSP)
}

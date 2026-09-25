package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/devices"
)

// TestCloseQuitsWhenThereIsNoTray: the test driver has no tray, and a
// window hidden with no way back would be a process nobody can stop.
func TestCloseQuitsWhenThereIsNoTray(t *testing.T) {
	u := testUI(t)
	require.False(t, hasTray(u.sh.App))
	u.loop.start(u)
	u.onClose()
	require.False(t, u.hiddenToTray)
	require.True(t, u.loop.stopped(), "the tick kept running after quit")
}

// TestATickWithNoServerIsLoggedAndTheLoopGoesOn: the resident process must
// survive the sound server being away, and say so once per tick.
func TestATickWithNoServerIsLoggedAndTheLoopGoesOn(t *testing.T) {
	u := testUI(t)
	require.NoError(t, config.Save("", func() config.Config { c := config.Default(); c.AutoSwitch = true; return c }()))
	u.loop.fire(u)
	require.Equal(t, 1, u.loop.fired)
	u.loop.fire(u)
	require.Equal(t, 2, u.loop.fired)
}

// TestTheLoopStopsWhenHalted: no goroutine outlives the window.
func TestTheLoopStopsWhenHalted(t *testing.T) {
	u := testUI(t)
	u.loop.start(u)
	require.False(t, u.loop.stopped())
	u.loop.halt()
	require.True(t, u.loop.stopped())
	u.loop.halt() // twice is fine
	time.Sleep(10 * time.Millisecond)
}

// TestTheRowActionsFollowTheSelection: nothing is enabled until a row is
// picked; a playing device offers no switch; an away Bluetooth device
// offers Connect and Switch; a connected one offers Disconnect.
func TestTheRowActionsFollowTheSelection(t *testing.T) {
	u := testUI(t)
	u.statusOK = true
	u.status = core.StatusResult{
		Server: audio.Server{Name: "fake", DefaultSink: "a"},
		Devices: []devices.Device{
			{ID: "a", Name: "Speakers", Sink: "a", Online: true, Connected: true, Default: true},
			{ID: "b", Name: "Headset", Sink: "b", Online: true, Connected: true},
			{ID: "bt:AA:BB:CC:DD:EE:FF", Name: "AirPods [Disconnected]", MAC: "AA:BB:CC:DD:EE:FF"},
			{ID: "bt:00:11:22:33:44:55", Name: "Buds", Sink: "bluez_output.x", MAC: "00:11:22:33:44:55", Online: true, Connected: true},
		},
	}
	body := u.buildOutputs()
	sw, con, dis := fynetest.FindButton(body, "Switch to"), fynetest.FindButton(body, "Connect"), fynetest.FindButton(body, "Disconnect")
	require.True(t, sw.Disabled() && con.Disabled() && dis.Disabled(), "an action was enabled with nothing selected")

	for i, want := range []struct{ sw, con, dis bool }{
		{false, false, false}, // playing already
		{true, false, false},  // a wired device that can play
		{true, true, false},   // away Bluetooth: connect, or switch which connects first
		{false, false, true},  // connected Bluetooth, not playing: it can be switched to as well
	} {
		u.selected = i
		body = u.buildOutputs()
		sw, con, dis = fynetest.FindButton(body, "Switch to"), fynetest.FindButton(body, "Connect"), fynetest.FindButton(body, "Disconnect")
		if i == 3 {
			want.sw = true
		}
		require.Equalf(t, want.sw, !sw.Disabled(), "row %d: Switch to", i)
		require.Equalf(t, want.con, !con.Disabled(), "row %d: Connect", i)
		require.Equalf(t, want.dis, !dis.Disabled(), "row %d: Disconnect", i)
	}
}

// TestTheAutoSwitchCheckWritesTheSetting: the check is the setting, and the
// CLI reads the same file.
func TestTheAutoSwitchCheckWritesTheSetting(t *testing.T) {
	u := testUI(t)
	u.statusOK = true
	u.status = core.StatusResult{Server: audio.Server{Name: "fake"}, Config: config.Default()}
	body := u.buildOutputs()
	check := fynetest.FindCheck(body)
	require.False(t, check.Checked)
	check.SetChecked(true)
	cfg, _, err := config.Load("")
	require.NoError(t, err)
	require.True(t, cfg.AutoSwitch, "the setting was not written")
	require.True(t, u.status.Config.AutoSwitch)
}

// TestARequestFromASecondInvocationIsAnswered: "show" brings the window
// back, an unknown verb is refused, and a volume step with no server says
// why in the reply the caller prints.
func TestARequestFromASecondInvocationIsAnswered(t *testing.T) {
	u := testUI(t)
	u.hiddenToTray = true
	require.Equal(t, "ok shown", u.handle("show"))
	require.False(t, u.hiddenToTray)
	require.Contains(t, u.handle("dance"), "error unknown request")
	reply := u.handle("vol-up")
	require.True(t, strings.HasPrefix(reply, "error "), reply)
}

// TestSwitchedTextNamesTheRouting: the banner says through what and with
// which microphone, because those are the two things that go wrong.
func TestSwitchedTextNamesTheRouting(t *testing.T) {
	res := core.SwitchResult{Device: devices.Device{Name: "Headset"}, ViaJamesDSP: true, MicName: "Headset Mic"}
	require.Equal(t, "Playing on Headset through JamesDSP. Input: Headset Mic.", switchedText(res))
	res = core.SwitchResult{Device: devices.Device{Name: "Headset"}, Fallback: "no ports"}
	require.Equal(t, "Playing on Headset (JamesDSP could not be rewired: no ports).", switchedText(res))
}

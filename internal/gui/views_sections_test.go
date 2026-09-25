package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/devices"
	"github.com/ushineko/ototo/internal/loopback"
)

func loaded(u *ui) {
	u.statusOK = true
	u.status = core.StatusResult{
		Server: audio.Server{Name: "fake", DefaultSink: "a"},
		Config: config.Default(),
		Devices: []devices.Device{
			{ID: "a", Name: "Speakers", Sink: "a", Online: true, Connected: true, Default: true, Volume: 40},
			{ID: "b", Name: "Headset", Sink: "b", Online: true, Connected: true},
		},
		Sources: []core.Source{{Name: "src-a", Description: "Desk Mic"}, {Name: "src-b", Description: "Headset Mic"}},
	}
}

// TestMoveWritesTheWholeOrderAndFollowsTheRow: a device the user never
// ranked gets a place the first time anything moves, and the selection
// stays on the device that moved.
func TestMoveWritesTheWholeOrderAndFollowsTheRow(t *testing.T) {
	u := testUI(t)
	loaded(u)
	u.selected = 1
	body := u.buildOutputs()
	up := fynetest.FindButton(body, "Move up")
	require.False(t, up.Disabled())
	require.True(t, fynetest.FindButton(body, "Move down").Disabled(), "the last row can move down")
	u.moveSelected(-1)
	cfg, _, err := config.Load("")
	require.NoError(t, err)
	require.Equal(t, []string{"b", "a"}, cfg.DevicePriority)
	require.Equal(t, 0, u.selected)
	require.True(t, u.statusOK, "the list was dropped after a move; it should stay and be read again behind")
	require.Equal(t, "b", u.status.Devices[0].ID, "the row did not move at once")
}

// TestReorderedKeepsWhatTheOrderDoesNotName: a device the order does not
// list (one that appeared since) stays, after the named ones.
func TestReorderedKeepsWhatTheOrderDoesNotName(t *testing.T) {
	list := []devices.Device{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := reordered(list, []string{"c", "a", "zz"})
	require.Equal(t, []string{"c", "a", "b"}, currentOrder(got))
}

// TestTheMicrophoneChoiceIsWrittenAsMade: the row's Select is the setting;
// auto is the absence of a key, a source is stored by name and shown by
// description.
func TestTheMicrophoneChoiceIsWrittenAsMade(t *testing.T) {
	u := testUI(t)
	loaded(u)
	body := u.buildMicrophone()
	selects := fynetest.All[*widget.Select](body)
	require.Len(t, selects, 2)
	require.Equal(t, micChoiceAuto, selects[0].Selected)

	selects[1].SetSelected("Headset Mic")
	cfg, _, err := config.Load("")
	require.NoError(t, err)
	require.Equal(t, "src-b", cfg.MicLinks["b"])

	selects[1].SetSelected(micChoiceDefault)
	cfg, _, err = config.Load("")
	require.NoError(t, err)
	require.Equal(t, config.MicDefault, cfg.MicLinks["b"])

	require.Equal(t, "src-gone", micChoice("src-gone", u.status.Sources), "a pinned input that is away must stay visible")
}

// TestASettingsSwitchWritesOneKey: three checks, three keys, and turning one
// off leaves the other two as they were.
func TestASettingsSwitchWritesOneKey(t *testing.T) {
	u := testUI(t)
	loaded(u)
	body := u.buildSettings()
	checks := fynetest.All[*widget.Check](body)
	require.Len(t, checks, 5, "three settings switches and two desktop steps")
	for _, c := range checks[:3] {
		require.True(t, c.Checked)
	}
	checks[1].SetChecked(false) // notifications
	cfg, _, err := config.Load("")
	require.NoError(t, err)
	require.False(t, cfg.SwitchNotifications)
	require.True(t, cfg.MoveStreams)
	require.True(t, cfg.OSDEnabled)
}

// TestTheHeadsetCardSaysWhatItCannotDo: no tool means no control, and the
// headset being off is a state the card names rather than hides.
func TestTheHeadsetCardSaysWhatItCannotDo(t *testing.T) {
	u := testUI(t)
	loaded(u)
	require.Contains(t, strings.Join(fynetest.Texts(u.headsetCard()), "\n"), "headsetcontrol is not installed")
	u.status.HeadsetTool = true
	texts := strings.Join(fynetest.Texts(u.headsetCard()), "\n")
	require.Contains(t, texts, "not detected")
	u.status.Headset = devices.Headset{Detected: true, Battery: "87%"}
	require.Contains(t, strings.Join(fynetest.Texts(u.headsetCard()), "\n"), "87%")
	entry := fynetest.FindEntry(u.headsetCard())
	require.Equal(t, "0", entry.Text)
	entry.OnSubmitted("15")
	cfg, _, err := config.Load("")
	require.NoError(t, err)
	require.Equal(t, 15, cfg.ArctisIdleMinutes, "Enter did not write the timeout")
	entry.OnSubmitted("200")
	cfg, _, err = config.Load("")
	require.NoError(t, err)
	require.Equal(t, 15, cfg.ArctisIdleMinutes, "an out-of-range value was written")
}

// TestTheLoopbackCardNamesItsTier: no line-in is said plainly; with one,
// the card says whether systemd or this window runs it.
func TestTheLoopbackCardNamesItsTier(t *testing.T) {
	u := testUI(t)
	loaded(u)
	require.Contains(t, strings.Join(fynetest.Texts(u.loopbackCard()), "\n"), "No line-in source found")
	u.loopback = loopback.State{Source: "alsa_input.x-linein", Mode: loopback.ModeService, Active: true}
	card := u.loopbackCard()
	require.True(t, fynetest.FindCheck(card).Checked)
	require.Contains(t, strings.Join(fynetest.Tips(card), "\n"), loopback.ServiceName)
	u.loopback = loopback.State{Source: "alsa_input.x-linein", Mode: loopback.ModeDirect}
	card = u.loopbackCard()
	require.False(t, fynetest.FindCheck(card).Checked)
	require.Contains(t, strings.Join(fynetest.Tips(card), "\n"), "pw-loopback")
}

// TestTheVolumeCardShowsThePlayingDevice: the slider carries the level of
// the device marked as playing, and says which one.
func TestTheVolumeCardShowsThePlayingDevice(t *testing.T) {
	u := testUI(t)
	loaded(u)
	card := u.volumeCard()
	require.Contains(t, fynetest.Texts(card), "Volume: Speakers")
	require.Equal(t, 40.0, fynetest.FindSlider(card).Value)

	u.status.Devices[0].Default = false
	card = u.volumeCard()
	require.Contains(t, fynetest.Texts(card), "Nothing is playing.")
}

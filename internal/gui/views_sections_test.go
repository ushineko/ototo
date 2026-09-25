package gui

import (
	"testing"

	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/devices"
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
	require.Len(t, checks, 3)
	for _, c := range checks {
		require.True(t, c.Checked)
	}
	checks[1].SetChecked(false) // notifications
	cfg, _, err := config.Load("")
	require.NoError(t, err)
	require.False(t, cfg.SwitchNotifications)
	require.True(t, cfg.MoveStreams)
	require.True(t, cfg.OSDEnabled)
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

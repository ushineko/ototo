package gui

import (
	"testing"

	"fyne.io/fyne/v2/canvas"
	"github.com/stretchr/testify/require"
	"github.com/ushineko/fynedesygn/fynetest"

	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/devices"
	"github.com/ushineko/ototo/internal/notify"
)

// TestTheIndicatorShowsOnceForOneChange: the key's own write and the
// server's event for it are one change, and the second arrival shows
// nothing more. With the setting off nothing shows, but the state is kept,
// so turning it on does not replay an old value.
func TestTheIndicatorShowsOnceForOneChange(t *testing.T) {
	u := testUI(t)
	u.osd = newIndicator(u.sh.App)
	u.osd.tr.OnShow = nil // no compositor here
	defer u.osd.tr.Hide()

	u.osd.show(volumeSnapshot{percent: 45}, true)
	require.True(t, u.osd.tr.Shown())
	require.Equal(t, " 45 %", u.osd.value.Text)
	u.osd.tr.Hide()
	u.osd.show(volumeSnapshot{percent: 45}, true)
	require.False(t, u.osd.tr.Shown(), "the same snapshot showed again")

	u.osd.show(volumeSnapshot{percent: 50}, false)
	require.False(t, u.osd.tr.Shown(), "the indicator showed with the setting off")
	u.osd.show(volumeSnapshot{percent: 50}, true)
	require.False(t, u.osd.tr.Shown(), "an old value replayed when the setting came on")

	u.osd.show(volumeSnapshot{percent: 50, muted: true}, true)
	require.True(t, u.osd.tr.Shown())
	require.Equal(t, "muted", u.osd.value.Text)
}

// TestASwitchGoesToTheIndicatorOnlyWhenAsked: with the setting off the
// notification reaches the desktop; on, the indicator shows the device and
// nothing is sent. A failure always reaches the desktop.
func TestASwitchGoesToTheIndicatorOnlyWhenAsked(t *testing.T) {
	u := testUI(t)
	u.osd = newIndicator(u.sh.App)
	u.osd.tr.OnShow = nil
	defer u.osd.tr.Hide()
	rec := &notify.Recorder{}
	r := routingNotifier{u: u, bus: rec}
	u.statusOK = true
	u.status = core.StatusResult{Config: config.Default()}

	_, _ = r.Send(notify.Notification{Kind: notify.KindSwitched, Title: "Audio Switched", Body: "Output: Headset\nInput: Mic"})
	require.Len(t, rec.Sent, 1)
	require.False(t, u.osd.tr.Shown())

	u.status.Config.SwitchInOSD = true
	_, _ = r.Send(notify.Notification{Kind: notify.KindSwitched, Title: "Audio Switched", Body: "Output: Headset\nInput: Mic"})
	require.Len(t, rec.Sent, 1, "a switch was notified although the indicator shows it")

	_, _ = r.Send(notify.Notification{Kind: notify.KindFailure, Title: "Switch Failed"})
	require.Len(t, rec.Sent, 2, "a failure did not reach the desktop")
}

// TestTheIndicatorNamesTheDevice: the panel says which output it is about,
// so a switch to another device at the same level is a change worth
// showing, and the name comes from the list.
func TestTheIndicatorNamesTheDevice(t *testing.T) {
	u := testUI(t)
	u.osd = newIndicator(u.sh.App)
	u.osd.tr.OnShow = nil
	defer u.osd.tr.Hide()
	u.statusOK = true
	u.status = core.StatusResult{Config: config.Default(), Devices: []devices.Device{{Sink: "a", Name: "Speakers"}}}
	require.Equal(t, "Speakers", u.deviceName("a"))
	require.Equal(t, "alsa_output.b", u.deviceName("alsa_output.b"))

	u.showVolume(core.VolumeResult{Sink: "a", Percent: 40})
	require.Equal(t, "Speakers", u.osd.device.Text)
	u.osd.tr.Hide()
	u.showVolume(core.VolumeResult{Sink: "alsa_output.b", Percent: 40})
	require.True(t, u.osd.tr.Shown(), "the same level on another device did not show")
	require.Equal(t, "alsa_output.b", u.osd.device.Text)
}

// TestTheIndicatorFontIsTheSettings: the window's font means no source of
// its own; a family that does not exist reads as the window's too, since a
// missing font is not a reason to draw nothing.
func TestTheIndicatorFontIsTheSettings(t *testing.T) {
	u := testUI(t)
	u.osd = newIndicator(u.sh.App)
	u.osd.draw(volumeSnapshot{percent: 40})
	require.Nil(t, u.osd.value.FontSource)
	u.statusOK = true
	u.status = core.StatusResult{Config: config.Default()}
	u.status.Config.OSDFont = "No Such Family Anywhere"
	u.showVolume(core.VolumeResult{Percent: 41})
	require.Nil(t, u.osd.value.FontSource)
	require.Equal(t, "No Such Family Anywhere", u.osd.font)
}

// TestThePreviewIsTheIndicatorItself: the chooser's sample carries the
// number, the device and the size set, so what is chosen is what will show.
func TestThePreviewIsTheIndicatorItself(t *testing.T) {
	testUI(t)
	obj := indicatorPreview(nil, 48, "Headset")
	texts := fynetest.Texts(obj)
	require.Contains(t, texts, " 57 %")
	require.Contains(t, texts, "Headset")
	for _, txt := range fynetest.All[*canvas.Text](obj) {
		if txt.Text == " 57 %" {
			require.Equal(t, float32(48), txt.TextSize)
			require.Nil(t, txt.FontSource, "the window's font has no source of its own")
		}
	}
}

// TestTheValueTextTakesTheSettingsSize: the size is the setting, applied at
// the next show, and the default is the large one.
func TestTheValueTextTakesTheSettingsSize(t *testing.T) {
	u := testUI(t)
	u.osd = newIndicator(u.sh.App)
	u.osd.draw(volumeSnapshot{percent: 40})
	require.Equal(t, float32(config.DefaultOSDTextSize), u.osd.value.TextSize)
	u.statusOK = true
	u.status = core.StatusResult{Config: config.Default()}
	u.status.Config.OSDTextSize = 48
	u.showVolume(core.VolumeResult{Percent: 41})
	require.Equal(t, float32(48), u.osd.value.TextSize)
}

// TestTheFirstSnapshotMakesTheCardVisible: a glance card is hidden until
// its source answers; an indicator with a value is a source that answered.
// Shipped once as a 6 px strip, because nobody had said so.
func TestTheFirstSnapshotMakesTheCardVisible(t *testing.T) {
	u := testUI(t)
	u.osd = newIndicator(u.sh.App)
	require.False(t, u.osd.card.Drawn())
	u.osd.draw(volumeSnapshot{percent: 40})
	require.True(t, u.osd.card.Drawn())
	require.Greater(t, u.osd.win.Panel().Size().Height, float32(20))
}

// TestTheCaptionKeepsItsWidth: "5%" and "100%" must not resize the panel.
func TestTheCaptionKeepsItsWidth(t *testing.T) {
	u := testUI(t)
	u.osd = newIndicator(u.sh.App)
	u.osd.draw(volumeSnapshot{percent: 5})
	five := u.osd.value.Text
	u.osd.draw(volumeSnapshot{percent: 100})
	require.Len(t, u.osd.value.Text, len(five))
	u.osd.draw(volumeSnapshot{percent: 150})
	require.Len(t, u.osd.value.Text, len(five))
}

// TestTheSettingGatesTheIndicator: osdEnabled reads the status; before the
// first read the default applies.
func TestTheSettingGatesTheIndicator(t *testing.T) {
	u := testUI(t)
	require.True(t, u.osdEnabled())
	u.statusOK = true
	u.status = core.StatusResult{Config: config.Default()}
	u.status.Config.OSDEnabled = false
	require.False(t, u.osdEnabled())
}

package gui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/core"
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
	require.Equal(t, " 45 %", u.osd.meter.Caption())
	u.osd.tr.Hide()
	u.osd.show(volumeSnapshot{percent: 45}, true)
	require.False(t, u.osd.tr.Shown(), "the same snapshot showed again")

	u.osd.show(volumeSnapshot{percent: 50}, false)
	require.False(t, u.osd.tr.Shown(), "the indicator showed with the setting off")
	u.osd.show(volumeSnapshot{percent: 50}, true)
	require.False(t, u.osd.tr.Shown(), "an old value replayed when the setting came on")

	u.osd.show(volumeSnapshot{percent: 50, muted: true}, true)
	require.True(t, u.osd.tr.Shown())
	require.Equal(t, "muted", u.osd.meter.Caption())
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
	five := u.osd.meter.Caption()
	u.osd.draw(volumeSnapshot{percent: 100})
	require.Len(t, u.osd.meter.Caption(), len(five))
	u.osd.draw(volumeSnapshot{percent: 150})
	require.Len(t, u.osd.meter.Caption(), len(five))
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

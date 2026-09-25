package gui

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/glance"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/desktop"
)

// The indicator's timing, as the original's: shown for 1.5 s after the last
// change, and a server event is read 80 ms after it arrives so a burst of
// changes is one read.
const (
	indicatorHold     = glance.DefaultHold
	indicatorDebounce = 80 * time.Millisecond
	indicatorWidth    = 380
)

/*
indicator is the volume indicator (R8): a glance window with one meter,
shown for a moment when the volume changes and hidden on its own.

It draws a snapshot: percent and muted. Who calls it is the hotkey path,
which knows the value it just set, and the watcher, which reads the value
after the server reports a change. Both go through show, which compares the
snapshot with the last one shown, so a sink change that did not change the
volume shows nothing, and the program's own write shows once.
*/
type indicator struct {
	win   *glance.Window
	card  *glance.Card
	meter *glance.Meter
	tr    *glance.Transient
	// last is the snapshot on screen or last shown; shown says whether it
	// has been drawn at all.
	last  volumeSnapshot
	shown bool
}

type volumeSnapshot struct {
	percent int
	muted   bool
}

// newIndicator builds the window; it is not shown.
func newIndicator(a fyne.App) *indicator {
	w := glance.NewWindow(a, glance.Options{
		Title:     desktop.IndicatorTitle,
		MinWidth:  indicatorWidth,
		OnTop:     true,
		Secondary: true,
	})
	w.Window().SetIcon(appIcon())
	m := glance.NewMeter("Volume", 0)
	card := glance.NewCard("Volume")
	card.AddObject(m.Object())
	w.Panel().Add(card)
	in := &indicator{win: w, card: card, meter: m, tr: glance.NewTransient(w, indicatorHold)}
	in.tr.OnShow = func() {
		// Placed on the pointer's screen by the compositor; without KWin the
		// window stays where the compositor put it, which is not an error.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = desktop.PlaceIndicator(ctx)
	}
	return in
}

// draw sets the meter from a snapshot. The caption is padded to a fixed
// width so the panel does not change size between 5% and 100%.
func (in *indicator) draw(v volumeSnapshot) {
	fraction := float64(v.percent) / audio.MaxVolumePercent
	caption := glance.Pad(fmt.Sprintf("%d", v.percent), 3, "%", 1)
	st := fd.StatusGood
	switch {
	case v.muted:
		caption = "muted"
		st = fd.StatusInfo
	case v.percent > 100:
		st = fd.StatusWarn
	}
	in.meter.Set(fraction, caption, st)
	// A card draws once its source has answered; the first snapshot is the
	// answer. Without this the window is a 6 px strip with nothing in it.
	in.card.SetAvailable(true)
}

// show draws the snapshot and shows the window when it differs from the
// last one, on the UI thread. enabled false records the state and shows
// nothing, as the original did with the indicator turned off.
func (in *indicator) show(v volumeSnapshot, enabled bool) {
	same := in.shown && v == in.last
	in.last, in.shown = v, true
	if same || !enabled {
		return
	}
	in.draw(v)
	in.tr.Show()
}

/*
watchVolume is the subscription (R8.3): every sink change from the server
is read, debounced, and shown when the level or the mute changed. It runs
until the context ends; the window starts it and cancels it on quit.
*/
func (u *ui) watchVolume(ctx context.Context) {
	events := make(chan audio.Event, 64)
	go audio.Watch(ctx, u.server, events, func(s string) { u.events().Log(core.LevelWarn, s) })
	var timer *time.Timer
	for ev := range events {
		u.events().Log(core.LevelDebug, fmt.Sprintf("server event: %s %s %d", ev.Facility, ev.Change, ev.Index))
		if ev.Facility != audio.FacilitySink && !ev.Reconnected {
			continue
		}
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(indicatorDebounce, func() { u.volumeChanged(ctx) })
	}
	if timer != nil {
		timer.Stop()
	}
}

// volumeChanged reads the playing sink's volume and shows the indicator.
func (u *ui) volumeChanged(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, TickInterval)
	defer cancel()
	res, err := u.sw.Volume(ctx, core.VolumeRequest{Request: u.request()})
	if err != nil {
		u.events().Log(core.LevelWarn, "reading the volume for the indicator: "+err.Error())
		return
	}
	u.events().Log(core.LevelDebug, fmt.Sprintf("volume read: %s %d%% muted=%v", res.Sink, res.Percent, res.Muted))
	u.showVolume(res)
}

// showVolume hops to the UI thread with a volume result.
func (u *ui) showVolume(res core.VolumeResult) {
	fyne.Do(func() {
		if u.osd == nil {
			return
		}
		u.events().Log(core.LevelDebug, fmt.Sprintf("indicator: enabled=%v", u.osdEnabled()))
		u.osd.show(volumeSnapshot{percent: res.Percent, muted: res.Muted}, u.osdEnabled())
	})
}

// osdEnabled is the setting, from the last status read; before the first
// read the default (on) applies.
func (u *ui) osdEnabled() bool {
	if !u.statusOK {
		return true
	}
	return u.status.Config.OSDEnabled
}

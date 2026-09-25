package gui

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/glance"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/config"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/desktop"
	"github.com/ushineko/ototo/internal/notify"
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
	// value is the number, drawn large: the indicator is read from across
	// the room, and the meter's own caption is sized for a panel that is
	// read up close. Its size is the osd_text_size setting.
	value    *canvas.Text
	textSize float32
	// font is the family the value and the device draw in; "" is the
	// window's own. A family is read from its file and named to the text
	// directly, which is the one path Fyne resolves before any theme.
	// family, when set, is used instead of loading font: the preview draws
	// a family the chooser has already read.
	font   string
	family *fdtheme.Font
	// icon says at a glance what the panel is: a speaker for the volume,
	// an arrow for a switch. It is what tells this panel from a
	// notification.
	icon *canvas.Image
	tr   *glance.Transient
	// device names the output under the value, so a switch is the same
	// panel with a new name: one operation, nothing to collide with.
	device *canvas.Text
	// last is the snapshot on screen or last shown; shown says whether it
	// has been drawn at all.
	last  volumeSnapshot
	shown bool
}

type volumeSnapshot struct {
	percent int
	muted   bool
	device  string
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
	in := newIndicatorBody()
	in.win = w
	in.tr = glance.NewTransient(w, indicatorHold)
	w.Panel().Add(in.card)
	in.size()
	in.tr.OnShow = func() {
		// Placed on the pointer's screen by the compositor; without KWin the
		// window stays where the compositor put it, which is not an error.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = desktop.PlaceIndicator(ctx)
	}
	return in
}

// newIndicatorBody is the panel's content: the card with the icon, the
// value, the meter and the device name. It is what the window shows and
// what the font chooser previews, so the preview is the indicator itself.
func newIndicatorBody() *indicator {
	m := glance.NewMeter("", 0)
	value := canvas.NewText("", widgets.StatusColor(fd.StatusGood))
	value.Alignment = fyne.TextAlignCenter
	value.TextStyle = fyne.TextStyle{Bold: true}
	value.TextSize = config.DefaultOSDTextSize
	icon := canvas.NewImageFromResource(theme.VolumeUpIcon())
	icon.FillMode = canvas.ImageFillContain
	device := canvas.NewText("", theme.Color(theme.ColorNameForeground))
	device.Alignment = fyne.TextAlignCenter
	card := glance.NewCard("ototo")
	card.AddObject(container.NewBorder(nil, nil, icon, nil, container.NewVBox(value, m.Object(), device)))
	return &indicator{card: card, meter: m, value: value, icon: icon, device: device,
		textSize: config.DefaultOSDTextSize}
}

/*
indicatorPreview draws the indicator as it would look in a family at a
size, for the font chooser: the same body, a level and the playing device.
family nil is the window's font.
*/
func indicatorPreview(family *fdtheme.Font, size float32, device string) fyne.CanvasObject {
	in := newIndicatorBody()
	in.textSize = size
	in.family = family
	in.draw(volumeSnapshot{percent: 57, device: device})
	return in.card.Object()
}

// size applies the text size to the value and scales the icon with it.
func (in *indicator) size() {
	if in.textSize <= 0 {
		in.textSize = config.DefaultOSDTextSize
	}
	in.value.TextSize = in.textSize
	in.device.TextSize = max(in.textSize*0.45, 11)
	side := in.textSize * 1.6
	in.icon.SetMinSize(fyne.NewSize(side, side))
	family := in.family
	if family == nil {
		family = fdtheme.LoadFont(in.font) // nil for "" and for a family that will not read
	}
	in.value.FontSource = family.Face(fyne.TextStyle{Bold: true})
	in.device.FontSource = family.Face(fyne.TextStyle{})
}

// draw sets the meter from a snapshot. The caption is padded to a fixed
// width so the panel does not change size between 5% and 100%.
func (in *indicator) draw(v volumeSnapshot) {
	fraction := float64(v.percent) / audio.MaxVolumePercent
	caption := glance.Pad(fmt.Sprintf("%d", v.percent), 3, "%", 1)
	st := fd.StatusGood
	icon := theme.VolumeUpIcon()
	switch {
	case v.muted:
		caption = "muted"
		st = fd.StatusInfo
		icon = theme.VolumeMuteIcon()
	case v.percent > 100:
		st = fd.StatusWarn
	case v.percent < 34:
		icon = theme.VolumeDownIcon()
	}
	in.meter.Set(fraction, "", st)
	in.size()
	in.value.Text = caption
	in.value.Color = widgets.StatusColor(st)
	in.value.Refresh()
	in.device.Text = v.device
	in.device.Refresh()
	in.icon.Resource = icon
	in.icon.Refresh()
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
		u.showTextSize()
		u.osd.show(volumeSnapshot{percent: res.Percent, muted: res.Muted, device: u.deviceName(res.Sink)}, u.osdEnabled())
	})
}

// deviceName is what the list calls a sink, else the sink's own name.
func (u *ui) deviceName(sink string) string {
	for _, d := range u.status.Devices {
		if d.Sink == sink {
			return d.Name
		}
	}
	return sink
}

// osdEnabled is the setting, from the last status read; before the first
// read the default (on) applies.
func (u *ui) osdEnabled() bool {
	if !u.statusOK {
		return true
	}
	return u.status.Config.OSDEnabled
}

/*
routingNotifier is the Switcher's notifier in the window: a switch goes to
the indicator when the setting says so, and everything else, and every
failure, goes to the desktop.
*/
type routingNotifier struct {
	u   *ui
	bus notify.Notifier
}

func (r routingNotifier) Send(n notify.Notification) (uint32, error) {
	if n.Kind == notify.KindSwitched && r.u.switchInOSD() {
		// The panel with the new device's name and volume: the same show a
		// key press gets, so nothing collides with it. The list is read
		// again first so the name is the new device's.
		go func() {
			r.u.refreshQuietly()
			r.u.volumeChanged(context.Background())
		}()
		return 0, nil
	}
	if r.bus == nil {
		return 0, nil
	}
	return r.bus.Send(n)
}

// switchInOSD is the setting, from the last status read.
func (u *ui) switchInOSD() bool {
	return u.statusOK && u.status.Config.SwitchInOSD
}

// showTextSize passes the text size and the font settings to the indicator.
func (u *ui) showTextSize() {
	if !u.statusOK || u.osd == nil {
		return
	}
	if u.status.Config.OSDTextSize > 0 {
		u.osd.textSize = float32(u.status.Config.OSDTextSize)
	}
	u.osd.font = u.status.Config.OSDFont
}

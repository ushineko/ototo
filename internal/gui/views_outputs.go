package gui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/forms"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/audio"

	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/devices"
)

// outputsTableHeight keeps the section still while a person reads it: a table
// takes all the space it receives.
const outputsTableHeight float32 = 360

/*
liveOutputs are the widgets of the Outputs section that a volume change
updates in place: the slider, the mute check and the table. A rebuild for
a step of the volume made the slider jump under the pointer and the section
flash; these are set in place instead, the way the design system asks.
*/
type liveOutputs struct {
	slider *forms.SliderEntry
	mute   *widget.Check
	table  *fyne.Container
}

/*
buildOutputs is the device list (R10.1, first half): every device in priority
order, the playing one marked, with the actions that act on the selected row.

The actions sit above the table and stay in position; the table is a fixed
height with its own scrollbar. A row action starts disabled and the
selection enables it, as the design system asks, and the builder is the
one place that knows why each button is enabled.
*/
func (u *ui) buildOutputs() fyne.CanvasObject {
	heading := widgets.Heading("Outputs", "Every device in your order, and which one is playing.")
	if !u.statusOK {
		return container.NewVBox(heading, widgets.Note("Reading the sound server…", fd.StatusInfo))
	}
	res := u.status
	if res.ServerError != "" {
		return container.NewVBox(heading,
			widgets.Note("No sound server answered: "+res.ServerError, fd.StatusBad),
			widgets.DimWrapped("ototo speaks the PulseAudio protocol, which PipeWire serves through "+
				"pipewire-pulse. Start it, or point --server at the socket, then press F5."),
		)
	}

	tw := u.deviceTable()
	if u.selected >= len(res.Devices) {
		u.selected = -1
	}

	switchBtn := widget.NewButtonWithIcon("Switch to", theme.MediaPlayIcon(), func() { u.switchToSelected() })
	switchBtn.Importance = widget.HighImportance
	connectBtn := widget.NewButtonWithIcon("Connect", theme.ConfirmIcon(), func() { u.connectSelected(true) })
	disconnectBtn := widget.NewButtonWithIcon("Disconnect", theme.CancelIcon(), func() { u.connectSelected(false) })

	upBtn := widget.NewButtonWithIcon("Move up", theme.MoveUpIcon(), func() { u.moveSelected(-1) })
	downBtn := widget.NewButtonWithIcon("Move down", theme.MoveDownIcon(), func() { u.moveSelected(+1) })
	enable := func() {
		switchBtn.Disable()
		connectBtn.Disable()
		disconnectBtn.Disable()
		upBtn.Disable()
		downBtn.Disable()
		if u.selected < 0 || u.selected >= len(res.Devices) {
			return
		}
		d := res.Devices[u.selected]
		if u.selected > 0 {
			upBtn.Enable()
		}
		if u.selected < len(res.Devices)-1 {
			downBtn.Enable()
		}
		if d.Online && d.Connected && !d.Default {
			switchBtn.Enable()
		}
		if d.MAC != "" && !d.Online {
			switchBtn.Enable() // connect, then switch (R5.5)
			connectBtn.Enable()
		}
		if d.MAC != "" && d.Online {
			disconnectBtn.Enable()
		}
	}
	enable()
	tw.OnSelected = func(id widget.TableCellID) {
		u.selected = id.Row
		enable()
	}
	tw.OnUnselected = func(widget.TableCellID) {
		u.selected = -1
		enable()
	}
	if u.selected >= 0 {
		tw.Select(widget.TableCellID{Row: u.selected})
	}

	auto := check("Switch automatically to the first available device", res.Config.AutoSwitch, func(on bool) {
		u.setAutoSwitch(on)
	})
	u.sh.Gate(switchBtn, connectBtn, disconnectBtn, upBtn, downBtn)

	return container.NewVBox(
		heading,
		widgets.Card("Sound server",
			widgets.FactRow("Server", res.Server.Name+" "+res.Server.Version, fd.StatusGood),
			widgets.FactRow("Default output", widgets.OrNone(res.Server.DefaultSink, "none"), fd.StatusInfo),
			widgets.FactRow("Default input", widgets.OrNone(res.Server.DefaultSource, "none"), fd.StatusInfo),
			widgets.FactRow("Inputs", fmt.Sprintf("%d", res.Inputs), fd.StatusInfo),
		),
		u.volumeCard(),
		container.NewHBox(
			widgets.WithTip(switchBtn, "Play on this device now. An away Bluetooth device is connected first."),
			widgets.WithTip(connectBtn, "Bring a Bluetooth device up without moving the audio to it. The order, "+
				"or Switch to, decides what plays."),
			widgets.WithTip(disconnectBtn, "Drop the Bluetooth connection. Its sink goes away and the order moves on."),
			widgets.Sep(), upBtn, downBtn),
		widgets.WithTip(auto, "Every five seconds, the highest device in your order that can play becomes the "+
			"output. Move devices up and down to change what wins."),
		widgets.FixedHeight(u.live.table, outputsTableHeight),
	)
}

// deviceTable builds the table from the list and keeps it in a container
// whose one child updateVolumeInPlace can swap.
func (u *ui) deviceTable() *widget.Table {
	t := table.New()
	t.Header("Device", "State", "Volume", "Id")
	for _, d := range u.status.Devices {
		t.Row(deviceStatus(d), deviceCells(d)...)
	}
	tw := t.Widget()
	u.live.table = container.NewStack(tw)
	return tw
}

/*
updateVolumeInPlace is a volume-only change on the section: the slider and
the mute take the playing device's values with their handlers held off,
and the table is swapped for one with the new column, in its container.
Nothing else moves, and the selection stays.
*/
func (u *ui) updateVolumeInPlace() {
	playing, ok := u.playingDevice()
	if !ok || u.live.slider == nil {
		return
	}
	u.live.slider.Set(float64(playing.Volume))
	if u.live.mute != nil {
		changed := u.live.mute.OnChanged
		u.live.mute.OnChanged = nil
		u.live.mute.SetChecked(playing.Mute)
		u.live.mute.OnChanged = changed
	}
	if u.live.table != nil {
		holder := u.live.table
		t := table.New()
		t.Header("Device", "State", "Volume", "Id")
		for _, d := range u.status.Devices {
			t.Row(deviceStatus(d), deviceCells(d)...)
		}
		tw := t.Widget()
		if old, ok := holder.Objects[0].(*widget.Table); ok {
			tw.OnSelected, tw.OnUnselected = old.OnSelected, old.OnUnselected
		}
		holder.Objects = []fyne.CanvasObject{tw}
		holder.Refresh()
		if u.selected >= 0 {
			tw.Select(widget.TableCellID{Row: u.selected})
		}
	}
}

/*
volumeCard is the playing device's level and mute (R10.1). The slider
commits once per gesture, so a drag is one write to the server rather than
four hundred, and the device it acts on is the hardware sink behind
JamesDSP when JamesDSP is the default (R6.3).
*/
func (u *ui) volumeCard() fyne.CanvasObject {
	playing, ok := u.playingDevice()
	if !ok {
		return widgets.Card("Volume", widgets.DimWrapped("Nothing is playing."))
	}
	slider := forms.NewSliderEntry(forms.SliderOptions{
		Min: 0, Max: audio.MaxVolumePercent, Step: 1, Value: float64(playing.Volume),
		Format: func(v float64) string { return fmt.Sprintf("%.0f", v) },
		Commit: func(v float64) { u.setVolume(int(v), nil) },
	})
	mute := check("Mute", playing.Mute, func(on bool) { u.setVolume(0, &on) })
	u.live.slider, u.live.mute = slider, mute
	return widgets.Card("Volume: "+playing.Name,
		container.NewBorder(nil, nil, nil, mute, slider.Widget()))
}

// playingDevice is the row whose volume the card shows: the default sink's,
// or JamesDSP's target when the default is JamesDSP, which the list marks
// as Default in either case.
func (u *ui) playingDevice() (devices.Device, bool) {
	for _, d := range u.status.Devices {
		if d.Default {
			return d, true
		}
	}
	return devices.Device{}, false
}

// setVolume writes a level or a mute to the playing sink, off the UI thread
// and not through the shell's loader: the loader rebuilds the section when
// the work starts and ends, and a rebuild is what made the slider jump.
func (u *ui) setVolume(percent int, mute *bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), TickInterval)
		defer cancel()
		if _, err := u.sw.SetVolume(ctx, core.SetVolumeRequest{Request: u.request(), Percent: percent, Mute: mute}); err != nil {
			fyne.Do(func() { u.sh.Report("Setting the volume", err) })
			return
		}
		u.refreshQuietly()
	}()
}

// moveSelected moves the selected device one place in the order and writes
// it. The selection follows the row.
func (u *ui) moveSelected(delta int) {
	d, ok := u.selectedDevice()
	if !ok {
		return
	}
	order := currentOrder(u.status.Devices)
	moved := core.Moved(order, d.ID, delta)
	u.sh.Perform("Saving the order...", func(ctx context.Context) error {
		cfg, err := core.SetPriority(ctx, core.SetPriorityRequest{Request: u.request(), Order: moved})
		fyne.Do(func() {
			if err != nil {
				return
			}
			// The list is reordered in place from what was written, so the
			// row moves at once; the read behind it confirms.
			u.status.Config = cfg
			u.status.Devices = reordered(u.status.Devices, moved)
			u.selected += delta
		})
		u.refreshQuietly()
		return err
	})
}

// reordered puts list in the order of ids, with anything not named kept
// after them in its old order.
func reordered(list []devices.Device, ids []string) []devices.Device {
	byID := make(map[string]devices.Device, len(list))
	for _, d := range list {
		byID[d.ID] = d
	}
	out := make([]devices.Device, 0, len(list))
	seen := map[string]bool{}
	for _, id := range ids {
		if d, ok := byID[id]; ok && !seen[id] {
			out = append(out, d)
			seen[id] = true
		}
	}
	for _, d := range list {
		if !seen[d.ID] {
			out = append(out, d)
		}
	}
	return out
}

// currentOrder is every listed device's id in list order, which is the
// priority order followed by the devices not yet ranked. Writing it whole
// is what the original did on a drag, and it means a device the user has
// never moved gets a place the first time any device moves.
func currentOrder(list []devices.Device) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, d.ID)
	}
	return out
}

// selectedDevice is the row the actions act on.
func (u *ui) selectedDevice() (devices.Device, bool) {
	if u.selected < 0 || u.selected >= len(u.status.Devices) {
		return devices.Device{}, false
	}
	return u.status.Devices[u.selected], true
}

// switchToSelected is a manual switch: it closes the breaker (R5.4) and
// reports the result as a banner.
func (u *ui) switchToSelected() {
	d, ok := u.selectedDevice()
	if !ok {
		return
	}
	u.sh.Perform("Switching to "+d.Name+"...", func(ctx context.Context) error {
		res, err := u.sw.Switch(ctx, core.SwitchRequest{Request: u.request(), Target: d.ID, Manual: true})
		// The list stays on screen and is read again behind it. Dropping
		// the loaded state here left "Reading the sound server…" in its
		// place until the next tick, which read as a pause of five seconds.
		u.refreshQuietly()
		fyne.Do(func() {
			if err == nil {
				u.sh.OK(switchedText(res))
			}
		})
		return err
	})
}

// switchedText says what a switch did, for the banner.
func switchedText(res core.SwitchResult) string {
	text := "Playing on " + res.Device.Name
	if res.ViaJamesDSP {
		text += " through JamesDSP"
	}
	if res.Fallback != "" {
		text += " (JamesDSP could not be rewired: " + res.Fallback + ")"
	}
	if res.MicName != "" {
		text += ". Input: " + res.MicName
	}
	return text + "."
}

// connectSelected connects or disconnects the selected Bluetooth device.
// A connect brings the device up and leaves the audio where it is; Switch
// to is the one that moves it (and connects first when it must).
func (u *ui) connectSelected(connect bool) {
	d, ok := u.selectedDevice()
	if !ok || d.MAC == "" {
		return
	}
	if connect {
		u.sh.Perform("Connecting "+d.Name+"...", func(ctx context.Context) error {
			dev, err := u.sw.Connect(ctx, core.ConnectRequest{Request: u.request(), Target: d.ID})
			u.refreshQuietly()
			fyne.Do(func() {
				if err == nil {
					u.sh.OK("Connected " + dev.Name + ". Switch to it, or let the order decide.")
				}
			})
			return err
		})
		return
	}
	u.sh.Perform("Disconnecting "+d.Name+"...", func(ctx context.Context) error {
		_, err := u.sw.Disconnect(ctx, core.DisconnectRequest{Request: u.request(), Target: d.ID})
		u.refreshQuietly()
		fyne.Do(func() {
			if err == nil {
				u.sh.OK("Disconnected " + d.Name + ".")
			}
		})
		return err
	})
}

// setAutoSwitch writes the setting. Off screen (a test), it runs inline.
func (u *ui) setAutoSwitch(on bool) {
	if u.status.Config.AutoSwitch == on {
		return
	}
	u.sh.Perform("Saving...", func(ctx context.Context) error {
		cfg, err := core.SetAutoSwitch(ctx, core.SetAutoSwitchRequest{Request: u.request(), Enabled: on})
		fyne.Do(func() {
			if err == nil {
				u.status.Config = cfg
			}
		})
		return err
	})
}

// deviceStatus colours a row: the playing device is good, one that cannot
// play is a warning, the rest are plain.
func deviceStatus(d devices.Device) fd.Status {
	switch {
	case !d.Connected:
		return fd.StatusWarn
	case d.Default:
		return fd.StatusGood
	default:
		return fd.StatusInfo
	}
}

// deviceCells is one table row, kept as a pure function because a table
// builds its cells only when it needs them and a test cannot reach them
// through the widget. The name already carries the port and the state the
// way the original wrote them, so State says only what a person acts on:
// away (no sink, connect it), disconnected (a sink with nothing plugged in),
// or ready.
func deviceCells(d devices.Device) []string {
	state := "ready"
	switch {
	case !d.Online:
		state = "away"
	case !d.Connected:
		state = "disconnected"
	case d.Default:
		state = "playing"
	}
	vol := ""
	switch {
	case !d.Online:
	case d.Mute:
		vol = "muted"
	default:
		vol = fmt.Sprintf("%d%%", d.Volume)
	}
	return []string{d.Name, state, vol, d.ID}
}

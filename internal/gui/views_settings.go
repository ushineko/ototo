package gui

import (
	"context"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/forms"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/core"
	"github.com/ushineko/ototo/internal/headset"
	"github.com/ushineko/ototo/internal/loopback"
)

/*
buildSettings is R10.4: the switches, each written as it is changed, with a
tip that says what it changes. The desktop steps (autostart, the
indicator's window rule, the volume keys) arrive with D9 and R11 and go
below these, each as a switch that says how it is undone.
*/
func (u *ui) buildSettings() fyne.CanvasObject {
	heading := widgets.Heading("Settings", "What ototo does on a switch, and what it shows.")
	if !u.statusOK {
		return container.NewVBox(heading, widgets.Note("Reading the settings…", fd.StatusInfo))
	}
	cfg := u.status.Config

	// Each check takes its value first and its handler second: SetChecked
	// fires OnChanged (quirk 2), and a handler that saved on build would
	// rebuild the section, which would save again, without end.
	move := check("Move playing audio to the new output", cfg.MoveStreams, func(on bool) {
		u.setSwitches(core.SetSwitchesRequest{MoveStreams: &on})
	})
	notes := check("Notify on an automatic switch", cfg.SwitchNotifications, func(on bool) {
		u.setSwitches(core.SetSwitchesRequest{SwitchNotifications: &on})
	})
	osd := check("Show the volume indicator", cfg.OSDEnabled, func(on bool) {
		u.setSwitches(core.SetSwitchesRequest{OSDEnabled: &on})
	})

	return container.NewVBox(
		heading,
		widgets.Card("Switching",
			widgets.WithTip(move, "Streams already playing follow the switch. Off, they keep playing where they "+
				"were until they restart."),
			widgets.WithTip(notes, "A desktop notification names the new output and input. A switch that fails is "+
				"always reported."),
		),
		widgets.Card("Volume indicator",
			widgets.WithTip(osd, "The small panel that appears when the volume changes. Off, the desktop's own "+
				"indicator is all you see."),
		),
		u.headsetCard(),
		u.loopbackCard(),
		u.desktopCard(),
		widgets.Card("Settings file",
			widgets.FactRow("Path", u.status.ConfigPath, fd.StatusInfo),
		),
	)
}

/*
headsetCard is R10.3: the Arctis battery and its idle timeout. Without
headsetcontrol the card says so and offers nothing; with the tool and the
headset off, the timeout can still be set, because the value is saved and
applied the next time the setting changes with the headset on.
*/
func (u *ui) headsetCard() fyne.CanvasObject {
	if !u.status.HeadsetTool {
		return widgets.Card("Headset (SteelSeries Arctis)",
			widgets.DimWrapped("headsetcontrol is not installed. With it, this card shows the battery and sets "+
				"the idle timeout."))
	}
	battery := widgets.FactRow("Battery", "not detected (off, or not plugged in)", fd.StatusWarn)
	if u.status.Headset.Detected {
		battery = widgets.FactRow("Battery", u.status.Headset.Battery, fd.StatusGood)
	}
	// The entry commits on Enter and not on every keystroke: each commit
	// runs headsetcontrol, and typing "15" is two values, not one.
	idle := widget.NewEntry()
	idle.Validator = forms.IntRange(0, headset.IdleMax)
	idle.SetText(strconv.Itoa(u.status.Config.ArctisIdleMinutes))
	idle.OnSubmitted = func(text string) {
		if m, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && m >= 0 && m <= headset.IdleMax {
			u.setHeadsetIdle(m)
		}
	}
	return widgets.Card("Headset (SteelSeries Arctis)",
		battery,
		widgets.WithTip(container.NewBorder(nil, nil, widget.NewLabel("Turn off after idle minutes"), nil,
			widgets.FixedWidth(idle, forms.NumericWidth)),
			"0 is never. The headset turns itself off after this long without sound, to save its battery. "+
				"Press Enter to apply, with the headset on."),
	)
}

func (u *ui) setHeadsetIdle(minutes int) {
	if u.status.Config.ArctisIdleMinutes == minutes {
		return
	}
	u.sh.Perform("Setting the headset's idle timeout...", func(ctx context.Context) error {
		cfg, err := core.SetHeadsetIdle(ctx, core.SetHeadsetIdleRequest{Request: u.request(), Minutes: minutes})
		fyne.Do(func() { u.status.Config = cfg })
		return err
	})
}

/*
loopbackCard is R9.3's control: play the line-in through the current
output. Without a line-in source the card says so and offers nothing. With
the systemd unit installed the card says the unit is what runs it, and the
check starts and stops the unit.
*/
func (u *ui) loopbackCard() fyne.CanvasObject {
	lb := u.loopback
	if u.status.ServerError != "" || lb.Mode == loopback.ModeNone {
		return widgets.Card("Line-in loopback",
			widgets.DimWrapped("No line-in source found. With one, this card plays it through the current output."))
	}
	how := "Runs pw-loopback from this window while it is on; remembered across restarts."
	if lb.Mode == loopback.ModeService {
		how = "Runs through the " + loopback.ServiceName + " user unit, which systemd remembers."
	}
	c := check("Play the line-in through the current output", lb.Active, func(on bool) { u.setLoopback(on) })
	return widgets.Card("Line-in loopback",
		widgets.FactRow("Source", lb.Source, fd.StatusInfo),
		widgets.WithTip(c, how),
	)
}

func (u *ui) setLoopback(on bool) {
	if u.loopback.Active == on {
		return
	}
	u.sh.Perform("Setting the loopback...", func(ctx context.Context) error {
		st, err := u.sw.SetLoopback(ctx, core.SetLoopbackRequest{Request: u.request(), Enabled: on})
		fyne.Do(func() {
			if err == nil {
				u.loopback = st
				u.status.Config.LoopbackEnabled = on
			}
		})
		u.refreshQuietly()
		return err
	})
}

// restoreLoopback is the start-of-process step, off the UI thread.
func (u *ui) restoreLoopback() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), TickInterval*2)
		defer cancel()
		if _, err := u.sw.RestoreLoopback(ctx, core.LoopbackRequest{Request: u.request()}); err != nil {
			u.events().Log(core.LevelWarn, "restoring the loopback: "+err.Error())
		}
	}()
}

/*
desktopCard is D9 and R8.5: what ototo installs into the desktop, each a
switch that says what it changes and how it is undone. Nothing here runs
on its own; the person turns it on.
*/
func (u *ui) desktopCard() fyne.CanvasObject {
	dt := u.desktop
	autostart := check("Start ototo at login", dt.Autostart, func(on bool) {
		u.setDesktop(core.SetDesktopRequest{Autostart: &on})
	})
	rule := check("Install the indicator's window rule (KDE Plasma)", dt.IndicatorRule, func(on bool) {
		u.setDesktop(core.SetDesktopRequest{IndicatorRule: &on})
	})
	rows := []fyne.CanvasObject{
		widgets.WithTip(autostart, "Writes one desktop entry under your autostart directory, and removes it "+
			"when turned off."),
		widgets.WithTip(rule, "Writes one rule into kwinrulesrc so the indicator has no titlebar, stays above "+
			"other windows, stays out of the taskbar and never takes the focus. Turned off, the rule is removed. "+
			"Without it the indicator still appears, with a titlebar."),
	}
	for _, e := range dt.Errors {
		rows = append(rows, widgets.Note(e, fd.StatusWarn))
	}
	return widgets.Card("Desktop", rows...)
}

func (u *ui) setDesktop(req core.SetDesktopRequest) {
	req.Request = u.request()
	u.sh.Perform("Changing the desktop...", func(ctx context.Context) error {
		st, err := core.SetDesktop(ctx, req)
		fyne.Do(func() { u.desktop = st })
		return err
	})
}

func (u *ui) setSwitches(req core.SetSwitchesRequest) {
	req.Request = u.request()
	u.sh.Perform("Saving...", func(ctx context.Context) error {
		cfg, err := core.SetSwitches(ctx, req)
		fyne.Do(func() {
			if err == nil {
				u.status.Config = cfg
			}
		})
		return err
	})
}

// check is a check box that holds a value before it has a handler, so the
// value it starts with is never mistaken for a change (quirk 2).
func check(label string, on bool, changed func(bool)) *widget.Check {
	c := widget.NewCheck(label, nil)
	c.SetChecked(on)
	c.OnChanged = changed
	return c
}

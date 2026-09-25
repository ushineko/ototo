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

	move := widget.NewCheck("Move playing audio to the new output", func(on bool) {
		u.setSwitches(core.SetSwitchesRequest{MoveStreams: &on})
	})
	move.SetChecked(cfg.MoveStreams)
	notes := widget.NewCheck("Notify on an automatic switch", func(on bool) {
		u.setSwitches(core.SetSwitchesRequest{SwitchNotifications: &on})
	})
	notes.SetChecked(cfg.SwitchNotifications)
	osd := widget.NewCheck("Show the volume indicator", func(on bool) {
		u.setSwitches(core.SetSwitchesRequest{OSDEnabled: &on})
	})
	osd.SetChecked(cfg.OSDEnabled)

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

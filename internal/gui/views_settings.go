package gui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/core"
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
		widgets.Card("Settings file",
			widgets.FactRow("Path", u.status.ConfigPath, fd.StatusInfo),
		),
	)
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

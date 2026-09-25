package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/devices"
)

// outputsTableHeight keeps the section still while a person reads it: a table
// takes all the space it receives.
const outputsTableHeight float32 = 360

// buildOutputs is what the sound server can see: the server, its defaults,
// and every output with its state.
func (u *ui) buildOutputs() fyne.CanvasObject {
	if !u.statusOK {
		return container.NewVBox(
			widgets.Heading("Outputs", "Every output the sound server can see, and which one is playing."),
			widgets.Note("Reading the sound server…", fd.StatusInfo),
		)
	}
	res := u.status
	if res.ServerError != "" {
		return container.NewVBox(
			widgets.Heading("Outputs", "Every output the sound server can see, and which one is playing."),
			widgets.Note("No sound server answered: "+res.ServerError, fd.StatusBad),
			widgets.DimWrapped("ototo speaks the PulseAudio protocol, which PipeWire serves through "+
				"pipewire-pulse. Start it, or point --server at the socket, then press F5."),
		)
	}

	t := table.New()
	t.Header("", "Device", "State", "Volume", "Id")
	for _, d := range res.Devices {
		t.Row(deviceStatus(d), deviceCells(d)...)
	}

	return container.NewVBox(
		widgets.Heading("Outputs", "Every output the sound server can see, and which one is playing."),
		widgets.Card("Sound server",
			widgets.FactRow("Server", res.Server.Name+" "+res.Server.Version, fd.StatusGood),
			widgets.FactRow("Default output", widgets.OrNone(res.Server.DefaultSink, "none"), fd.StatusInfo),
			widgets.FactRow("Default input", widgets.OrNone(res.Server.DefaultSource, "none"), fd.StatusInfo),
			widgets.FactRow("Inputs", fmt.Sprintf("%d", res.Inputs), fd.StatusInfo),
		),
		widgets.FixedHeight(t.Widget(), outputsTableHeight),
	)
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
	mark := ""
	if d.Default {
		mark = "playing"
	}
	state := "ready"
	switch {
	case !d.Online:
		state = "away"
	case !d.Connected:
		state = "disconnected"
	}
	vol := ""
	switch {
	case !d.Online:
	case d.Mute:
		vol = "muted"
	default:
		vol = fmt.Sprintf("%d%%", d.Volume)
	}
	return []string{mark, d.Name, state, vol, d.ID}
}

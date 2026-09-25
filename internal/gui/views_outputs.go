package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/table"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/core"
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
	t.Header("", "Output", "State", "Volume", "Name")
	for _, o := range res.Outputs {
		t.Row(outputStatus(o), outputCells(o)...)
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

// outputStatus colours a row: the default output is good, a disconnected one
// is a warning, the rest are plain.
func outputStatus(o core.Output) fd.Status {
	switch {
	case !o.Connected:
		return fd.StatusWarn
	case o.Default:
		return fd.StatusGood
	default:
		return fd.StatusInfo
	}
}

// outputCells is one table row, kept as a pure function because a table
// builds its cells only when it needs them and a test cannot reach them
// through the widget.
func outputCells(o core.Output) []string {
	mark := ""
	if o.Default {
		mark = "playing"
	}
	state := "connected"
	if !o.Connected {
		state = "disconnected"
	}
	vol := fmt.Sprintf("%d%%", o.Volume)
	if o.Mute {
		vol = "muted"
	}
	return []string{mark, widgets.OrNone(o.Description, o.Name), state, vol, o.Name}
}

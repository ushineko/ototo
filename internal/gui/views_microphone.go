package gui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/core"
)

// The two link choices that are not a source, as the user reads them.
const (
	micChoiceAuto    = "Match automatically"
	micChoiceDefault = "Leave the input alone"
)

/*
buildMicrophone is R10.2: for every output, which input follows it. A row
is the output's name and a Select: match automatically (the default), leave
the input alone, or one named input. The choice is written as it is made;
there is no Save, because one row is one setting and a person changes one
at a time.
*/
func (u *ui) buildMicrophone() fyne.CanvasObject {
	heading := widgets.Heading("Microphone", "Which input follows each output when it starts playing.")
	if !u.statusOK {
		return container.NewVBox(heading, widgets.Note("Reading the sound server…", fd.StatusInfo))
	}
	if u.status.ServerError != "" {
		return container.NewVBox(heading, widgets.Note("No sound server answered: "+u.status.ServerError, fd.StatusBad))
	}
	if len(u.status.Devices) == 0 {
		return container.NewVBox(heading, widgets.DimWrapped("No outputs yet. Once the sound server shows one, it appears here."))
	}

	options := micOptions(u.status.Sources)
	rows := make([]fyne.CanvasObject, 0, len(u.status.Devices))
	for _, d := range u.status.Devices {
		id := d.ID
		sel := widget.NewSelect(options, nil)
		sel.SetSelected(micChoice(u.status.Config.MicLinks[id], u.status.Sources))
		sel.OnChanged = func(choice string) { u.setMicLink(id, micLink(choice, u.status.Sources)) }
		rows = append(rows, container.NewBorder(nil, nil, widgets.FixedWidth(widget.NewLabel(d.Name), 360), nil, sel))
	}
	return container.NewVBox(
		heading,
		widgets.DimWrapped("Match automatically picks the input on the same device as the output: a headset's "+
			"own microphone, a Bluetooth device's own. Leave the input alone keeps whatever is set."),
		widgets.Card("Outputs", rows...),
	)
}

// micOptions is the Select's list: the two choices, then every input by
// its description.
func micOptions(sources []core.Source) []string {
	out := []string{micChoiceAuto, micChoiceDefault}
	for _, s := range sources {
		out = append(out, s.Description)
	}
	return out
}

// micChoice is what the Select shows for a stored link.
func micChoice(link string, sources []core.Source) string {
	switch link {
	case "", config.MicAuto:
		return micChoiceAuto
	case config.MicDefault:
		return micChoiceDefault
	}
	for _, s := range sources {
		if s.Name == link {
			return s.Description
		}
	}
	// A pinned input that is not present right now shows by name, so the
	// setting is visible rather than silently reading as automatic.
	return link
}

// micLink is the stored value for a choice.
func micLink(choice string, sources []core.Source) string {
	switch choice {
	case micChoiceAuto:
		return config.MicAuto
	case micChoiceDefault:
		return config.MicDefault
	}
	for _, s := range sources {
		if s.Description == choice {
			return s.Name
		}
	}
	return choice
}

func (u *ui) setMicLink(device, link string) {
	if current := u.status.Config.MicLinks[device]; (current == "" && link == config.MicAuto) || current == link {
		return
	}
	u.sh.Perform("Saving...", func(ctx context.Context) error {
		cfg, err := core.SetMicLink(ctx, core.SetMicLinkRequest{Request: u.request(), Device: device, Link: link})
		fyne.Do(func() {
			if err == nil {
				u.status.Config = cfg
			}
		})
		return err
	})
}

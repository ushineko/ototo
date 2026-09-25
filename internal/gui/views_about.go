package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"github.com/ushineko/fynedesygn/markdown"
	"github.com/ushineko/fynedesygn/mermaid"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	ototo "github.com/ushineko/ototo"
)

// projectURL is where the README lives. It is the one place this window sends a
// user outside itself, and it opens in the desktop's browser.
const projectURL = "https://github.com/ushineko/ototo"

/*
buildAbout is what ototo is and what it will and will not do, in the
library's About shape, with the README itself below the facts: the
document, with its routing diagram, rather than a shortened copy that would
drift. The pane follows the shell's scroller and is released when the
section is replaced.
*/
func (u *ui) buildAbout() fyne.CanvasObject {
	a := u.about()
	a.Extra = func(s *shell.Shell) fyne.CanvasObject {
		u.readme = markdown.New(ototo.README(), markdown.Options{
			FS:           ototo.Docs(),
			Diagrams:     mermaid.NewSet(ototo.Docs(), "diagrams"),
			SettleResize: 120 * time.Millisecond,
		})
		u.readme.Follow(s.Scroller())
		return u.readme
	}
	return shell.AboutSection(a).Build(u.sh)
}

// detachAbout releases the README pane's hold on the scroller.
func (u *ui) detachAbout() {
	if u.readme != nil {
		u.readme.Detach()
		u.readme = nil
	}
}

// about describes this program for the About section.
func (u *ui) about() shell.About {
	return shell.About{
		Icon:    appIcon(),
		Name:    "ototo",
		Version: u.version,
		Blurb: "Keeps your audio on the output you want. Switches to the highest-priority " +
			"connected device, follows it with the matching microphone, connects Bluetooth " +
			"headsets, and reroutes JamesDSP so effects never drop out. Native Go over the " +
			"PulseAudio protocol.",
		URL:     projectURL,
		URLText: "Project documentation",
		Notes: []shell.Note{
			{Title: "Origin", Detail: "音跳び (oto-tobi, sound-hop), shortened; AKA the little bro " +
				"(弟, otōto) that follows your audio around."},
			{Title: "Simplicity", Detail: "ototo changes the default output and input, the volume, and " +
				"which output JamesDSP plays through. It writes one settings file of its own. " +
				"Anything it installs into the desktop is reversible."},
		},
		Facts: []shell.Fact{
			{Label: "Commit", Value: u.commit},
			{Label: "Sound server", Value: serverFact(u)},
			{Label: "Settings file", Value: widgets.OrNone(u.status.ConfigPath, "not read yet")},
			{Label: "Licence", Value: "MIT"},
		},
	}
}

func serverFact(u *ui) string {
	if !u.statusOK {
		return "not read yet"
	}
	if u.status.ServerError != "" {
		return "not reached"
	}
	return u.status.Server.Name + " " + u.status.Server.Version
}

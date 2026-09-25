package gui

import (
	"fyne.io/fyne/v2"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
)

// projectURL is where the README lives. It is the one place this window sends a
// user outside itself, and it opens in the desktop's browser.
const projectURL = "https://github.com/ushineko/ototo"

// buildAbout is what ototo is and what it will and will not do, in the
// library's About shape. Limitations stay in the README; a shortened copy here
// would only drift.
func (u *ui) buildAbout() fyne.CanvasObject {
	return shell.AboutSection(u.about()).Build(u.sh)
}

// about describes this program for the About section.
func (u *ui) about() shell.About {
	return shell.About{
		Icon:    appIcon(),
		Name:    "ototo",
		Version: u.version + " (" + u.commit + ")",
		Blurb: "Keeps your audio on the output you want. Switches to the highest-priority " +
			"connected device, follows it with the matching microphone, connects Bluetooth " +
			"headsets, and reroutes JamesDSP so effects never drop out. Native Go over the " +
			"PulseAudio protocol: no pactl, no Python.",
		URL:     projectURL,
		URLText: "Project documentation",
		Notes: []shell.Note{
			{Title: "The name", Detail: "音跳び (oto-tobi, sound-hop), shortened. The little brother " +
				"(弟, otōto) that follows your audio around."},
			{Title: "Touch little", Detail: "ototo changes the default output and input, the volume, and " +
				"which output JamesDSP plays through. It writes one settings file of its own. " +
				"Anything it installs into the desktop (autostart, a KWin rule, a key binding) " +
				"is an explicit step, and each one is reversible."},
		},
		Facts: []shell.Fact{
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

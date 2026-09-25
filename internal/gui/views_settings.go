package gui

import (
	"context"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/dialogs"
	"github.com/ushineko/fynedesygn/forms"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/ototo/internal/config"
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
	tell := u.switchTellRow(cfg)
	osd := check("Show the volume indicator", cfg.OSDEnabled, func(on bool) {
		u.setSwitches(core.SetSwitchesRequest{OSDEnabled: &on})
	})

	return container.NewVBox(
		heading,
		widgets.Card("Switching",
			widgets.WithTip(move, "Streams already playing follow the switch. Off, they keep playing where they "+
				"were until they restart."),
			tell,
		),
		widgets.Card("Volume indicator",
			widgets.WithTip(osd, "The small panel that appears when the volume changes. Off, the desktop's own "+
				"indicator is all you see."),
			u.osdSizeRow(),
			u.osdFontRow(),
		),
		u.soundCard(),
		u.headsetCard(),
		u.loopbackCard(),
		u.desktopCard(),
		widgets.Card("Settings file",
			widgets.FactRow("Path", u.status.ConfigPath, fd.StatusInfo),
		),
	)
}

// What ototo does on an automatic switch: one choice, kept as the two
// settings keys the original wrote (switch_notifications and switch_in_osd).
const (
	tellNothing      = "Say nothing"
	tellNotification = "Send a desktop notification"
	tellIndicator    = "Show it in the indicator"
)

func tellChoice(cfg config.Config) string {
	switch {
	case !cfg.SwitchNotifications:
		return tellNothing
	case cfg.SwitchInOSD:
		return tellIndicator
	default:
		return tellNotification
	}
}

// switchTellRow is the selector for what a switch shows.
func (u *ui) switchTellRow(cfg config.Config) fyne.CanvasObject {
	sel := widget.NewSelect([]string{tellNothing, tellNotification, tellIndicator}, nil)
	sel.SetSelected(tellChoice(cfg))
	sel.OnChanged = func(choice string) {
		if choice == tellChoice(u.status.Config) {
			return
		}
		notify, inOSD := choice != tellNothing, choice == tellIndicator
		u.setSwitches(core.SetSwitchesRequest{SwitchNotifications: &notify, SwitchInOSD: &inOSD})
	}
	return widgets.WithTip(container.NewBorder(nil, nil, widget.NewLabel("On an automatic switch"), nil, sel),
		"A notification names the new output and input. The indicator shows the new output and its volume "+
			"for a moment. A switch that fails is always a notification, whatever is chosen.")
}

// windowsFont is the chooser's entry for no font of the indicator's own.
const windowsFont = "The window's font"

// osdFontRow is the indicator's font: the window's, or a family chosen in
// the library's font dialog, which shows a sample before anything is
// applied.
func (u *ui) osdFontRow() fyne.CanvasObject {
	current := u.status.Config.OSDFont
	shown := current
	if shown == "" {
		shown = "the window's font"
	}
	choose := widget.NewButton("Choose...", func() {
		// The list leads with the window's own font, and the sample is the
		// indicator itself: its number, its meter and the playing device,
		// at the size set, in the family under the cursor.
		names := append([]string{windowsFont}, fdtheme.FontNames()...)
		shown := current
		if shown == "" {
			shown = windowsFont
		}
		size := float32(u.status.Config.OSDTextSize)
		if size <= 0 {
			size = config.DefaultOSDTextSize
		}
		device := "Speakers"
		if playing, ok := u.playingDevice(); ok {
			device = playing.Name
		}
		dialogs.ChooseFontWith(u.sh.Window, "Indicator font", names, shown, u.sh.Appearance(), false,
			func(name string, family *fdtheme.Font) fyne.CanvasObject {
				if name == windowsFont {
					family = nil
				}
				return indicatorPreview(family, size, device)
			},
			func(name string) {
				if name == windowsFont {
					name = ""
				}
				if name == current {
					return
				}
				u.setSwitches(core.SetSwitchesRequest{OSDFont: &name})
			})
	})
	reset := widget.NewButton("Use the window's font", func() {
		none := ""
		u.setSwitches(core.SetSwitchesRequest{OSDFont: &none})
	})
	if current == "" {
		reset.Disable()
	}
	return widgets.WithTip(container.NewBorder(nil, nil, widget.NewLabel("Indicator font"),
		container.NewHBox(choose, reset), widget.NewLabel(shown)),
		"The family the indicator's value and device name draw in. The next change of volume shows it.")
}

// osdSizeRow is the indicator's text size: a selector of the sizes offered,
// with a value from an older file shown as it is.
func (u *ui) osdSizeRow() fyne.CanvasObject {
	current := strconv.Itoa(u.status.Config.OSDTextSize)
	options := make([]string, 0, len(core.OSDTextSizes)+1)
	seen := false
	for _, n := range core.OSDTextSizes {
		options = append(options, strconv.Itoa(n))
		seen = seen || n == u.status.Config.OSDTextSize
	}
	if !seen {
		options = append(options, current)
	}
	size := widget.NewSelect(options, nil)
	size.SetSelected(current)
	size.OnChanged = func(text string) {
		if n, err := strconv.Atoi(text); err == nil && n != u.status.Config.OSDTextSize {
			u.setSwitches(core.SetSwitchesRequest{OSDTextSize: &n})
		}
	}
	return widgets.WithTip(container.NewBorder(nil, nil, widget.NewLabel("Indicator text size"), nil,
		widgets.FixedWidth(size, forms.NumericWidth)),
		"Points. The next change of volume shows it.")
}

/*
soundCard is the sound on a switch: the built-in chime, or a WAV file of
the person's own, and a button that plays it now so the choice can be
heard before a switch happens.
*/
func (u *ui) soundCard() fyne.CanvasObject {
	cfg := u.status.Config
	on := check("Play a sound when the output switches", cfg.SwitchSound, func(on bool) {
		u.setSwitches(core.SetSwitchesRequest{SwitchSound: &on})
	})
	file := widget.NewEntry()
	file.SetPlaceHolder("empty means the built-in chime")
	file.SetText(cfg.SwitchSoundFile)
	file.OnSubmitted = func(text string) {
		text = strings.TrimSpace(text)
		if text != u.status.Config.SwitchSoundFile {
			u.setSwitches(core.SetSwitchesRequest{SwitchSoundFile: &text})
		}
	}
	play := widget.NewButtonWithIcon("Play it now", theme.MediaPlayIcon(), func() {
		u.sh.Load("Playing...", func(context.Context) error {
			return core.PlaySwitchSound(u.server, "", 0, u.status.Config)
		})
	})
	// Commits on Enter, as the headset's idle entry does.
	delay := widget.NewEntry()
	delay.Validator = forms.IntRange(0, core.SwitchSoundDelayMax)
	delay.SetText(strconv.Itoa(cfg.SwitchSoundDelay))
	delay.OnSubmitted = func(text string) {
		if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && n != u.status.Config.SwitchSoundDelay {
			u.setSwitches(core.SetSwitchesRequest{SwitchSoundDelay: &n})
		}
	}
	return widgets.Card("Sound on a switch",
		widgets.WithTip(on, "Played on the device that just became the output, after the switch, so it "+
			"comes out of the new device."),
		widgets.WithTip(container.NewBorder(nil, nil, widget.NewLabel("Sound file"), play,
			dialogs.WithBrowse(u.sh.Window, file, false)),
			"A 16-bit PCM WAV file. Press Enter to apply; empty means the built-in chime."),
		widgets.WithTip(container.NewBorder(nil, nil, widget.NewLabel("After a device connects, wait seconds"), nil,
			widgets.FixedWidth(delay, forms.NumericWidth)),
			"Headphones that just connected take a while before they play anything, and their own "+
				"connect chime comes first; a sound played before that is lost. Sony WH-1000XM6 need about "+
				"ten seconds. Press Enter to apply."),
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
	rows := []fyne.CanvasObject{widgets.WithTip(c, how)}
	if len(lb.Candidates) > 1 {
		rows = append(rows, u.loopbackSourceRow(lb))
	} else {
		rows = append(rows, widgets.FactRow("Source", u.sourceName(lb.Source), fd.StatusInfo))
	}
	return widgets.Card("Line-in loopback", rows...)
}

// loopbackSourceRow is the selector for a machine with more than one line
// input: automatic (the first found) or one by name.
func (u *ui) loopbackSourceRow(lb loopback.State) fyne.CanvasObject {
	const automatic = "Automatic (the first found)"
	options := []string{automatic}
	for _, c := range lb.Candidates {
		options = append(options, u.sourceName(c))
	}
	sel := widget.NewSelect(options, nil)
	current := automatic
	if u.status.Config.LoopbackSource != "" {
		current = u.sourceName(u.status.Config.LoopbackSource)
	}
	sel.SetSelected(current)
	sel.OnChanged = func(choice string) {
		want := ""
		for _, c := range lb.Candidates {
			if u.sourceName(c) == choice {
				want = c
			}
		}
		if want == u.status.Config.LoopbackSource {
			return
		}
		u.setSwitches(core.SetSwitchesRequest{LoopbackSource: &want})
	}
	return widgets.WithTip(container.NewBorder(nil, nil, widget.NewLabel("Line-in source"), nil, sel),
		"Which line input to play. The choice takes effect the next time the loopback is turned on.")
}

// sourceName is what the list calls an input, else its own name.
func (u *ui) sourceName(name string) string {
	for _, s := range u.status.Sources {
		if s.Name == name {
			return s.Description
		}
	}
	return name
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
	keys := check("Use the volume keys for ototo (KDE Plasma)", dt.VolumeKeys, func(on bool) {
		u.setDesktop(core.SetDesktopRequest{VolumeKeys: &on})
	})
	rows := []fyne.CanvasObject{
		widgets.WithTip(autostart, "Writes one desktop entry under your autostart directory, and removes it "+
			"when turned off."),
		widgets.WithTip(keys, "Volume Up and Volume Down run ototo, which changes the volume and shows the "+
			"indicator. What held the keys before is recorded and released; turned off, ototo's shortcuts are "+
			"removed and the keys go back to it."),
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

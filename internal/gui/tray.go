package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// hasTray reports whether the platform offers a system tray. Fyne's desktop
// driver does; the test driver does not.
func hasTray(a fyne.App) bool {
	_, ok := a.(desktop.App)
	return ok
}

// setupTray installs the tray icon and its menu: Show, About, Quit, as the
// original's (R9.4).
func (u *ui) setupTray() {
	d, ok := u.sh.App.(desktop.App)
	if !ok {
		return
	}
	menu := fyne.NewMenu("ototo",
		fyne.NewMenuItem("Show", u.showWindow),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("About", func() { u.showWindow(); u.sh.Select("About") }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", u.quit),
	)
	d.SetSystemTrayMenu(menu)
	d.SetSystemTrayIcon(appIcon())
}

// showWindow brings the window back from the tray.
func (u *ui) showWindow() {
	u.hiddenToTray = false
	if u.sh.Window != nil {
		u.sh.Window.Show()
		u.sh.Window.RequestFocus()
	}
	u.loadStatus()
}

/*
onClose is the window's close intercept: hide to the tray so the switching
continues, as the original's closeEvent did. Without a tray the window
quits instead: hidden with no way back, it would be a process the user
cannot see and cannot stop.
*/
func (u *ui) onClose() {
	if !hasTray(u.sh.App) {
		u.quit()
		return
	}
	u.hiddenToTray = true
	u.sh.Window.Hide()
}

// quit stops the tick and the loopback child, and exits.
func (u *ui) quit() {
	u.loop.halt()
	u.sw.Close()
	if u.sh.App != nil {
		u.sh.App.Quit()
	}
}

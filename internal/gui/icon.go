package gui

import (
	"fyne.io/fyne/v2"

	"github.com/ushineko/ototo/internal/gui/assets"
)

// appIcon is the window and taskbar icon. The bytes live in
// internal/gui/assets, embedded at build time; packaging/ototo.svg is the same
// drawing again, on disk for the installer to place into the icon theme.
func appIcon() fyne.Resource { return fyne.NewStaticResource("ototo.svg", assets.IconSVG()) }

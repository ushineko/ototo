package gui

import (
	"fyne.io/fyne/v2"
	"github.com/ushineko/fynedesygn/shell"
)

// appearanceSample is the monospace text the font picker previews: what the
// program itself prints in a fixed-width face.
const appearanceSample = "  * alsa_output.usb-Yeti_Nano-00.analog-stereo   connected   73%"

// buildAppearance is the library's Appearance section: scheme, font, text
// size and scale, stored by the shell under the app ID.
func (u *ui) buildAppearance() fyne.CanvasObject {
	return shell.AppearanceSection(appearanceSample).Build(u.sh)
}

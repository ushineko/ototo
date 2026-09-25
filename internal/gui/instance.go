package gui

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"

	"github.com/ushineko/ototo/internal/audio"
	"github.com/ushineko/ototo/internal/core"
)

/*
handle answers a request from a second invocation (D10): "show" brings the
window back, "vol-up" and "vol-down" step the playing output, "connect X"
switches to X. The volume and connect requests run here, in the resident
process, so that the indicator it will draw (R8) is the one the person sees,
and so that the breaker and the last-switch state are the process's own.

The reply is one line: "ok" and what happened, or "error" and why. The
caller prints it and exits with its first word.
*/
func (u *ui) handle(request string) string {
	verb, arg, _ := strings.Cut(request, " ")
	ctx, cancel := context.WithTimeout(context.Background(), TickInterval*3)
	defer cancel()
	switch verb {
	case "show":
		fyne.Do(u.showWindow)
		return "ok shown"
	case "vol-up", "vol-down":
		delta := audio.VolumeStep
		if verb == "vol-down" {
			delta = -delta
		}
		res, err := u.sw.Volume(ctx, core.VolumeRequest{Request: u.request(), Delta: delta})
		if err != nil {
			return "error " + err.Error()
		}
		// The key's own change shows at once; the server's event for the
		// same write then finds the same snapshot and shows nothing more.
		u.showVolume(res)
		u.afterRequest()
		state := fmt.Sprintf("%d%%", res.Percent)
		if res.Muted {
			state = "muted"
		}
		return "ok " + res.Sink + ": " + state
	case "connect":
		res, err := u.sw.Switch(ctx, core.SwitchRequest{Request: u.request(), Target: arg, Manual: true})
		if err != nil {
			return "error " + err.Error()
		}
		u.afterRequest()
		return "ok " + switchedText(res)
	default:
		return "error unknown request " + verb
	}
}

// afterRequest reads the state again when the window can be seen.
func (u *ui) afterRequest() {
	fyne.Do(func() {
		if u.hiddenToTray || !u.sh.OnScreen() {
			return
		}
		u.statusOK = false
		u.loadStatus()
	})
}

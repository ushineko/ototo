package gui

import (
	"context"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"github.com/ushineko/ototo/internal/core"
)

// TickInterval is how often the resident process looks at the machine: the
// original's 5 s.
const TickInterval = 5 * time.Second

/*
ticker is the auto-switch loop (R5.3): every TickInterval, one AutoSwitch,
then a fresh read of the state for the window. A tick that finds the last one
still running is skipped rather than queued: a Bluetooth connect can take
longer than the interval.

The loop runs outside the shell's Perform, because it is not something the
user started and must not put a busy popup over the window every five
seconds. Its result reaches the window through the status reload, and its
log lines through Events.
*/
type ticker struct {
	mu      sync.Mutex
	stop    chan struct{}
	running bool
	// fired counts ticks, for tests.
	fired int
}

func (t *ticker) start(u *ui) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stop != nil {
		close(t.stop)
	}
	t.stop = make(chan struct{})
	stop := t.stop
	go func() {
		tick := time.NewTicker(TickInterval)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				t.fire(u)
			}
		}
	}()
}

func (t *ticker) stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stop == nil
}

func (t *ticker) halt() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stop != nil {
		close(t.stop)
		t.stop = nil
	}
}

// fire runs one tick.
func (t *ticker) fire(u *ui) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.fired++
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), TickInterval*3)
	defer cancel()
	res, err := u.sw.AutoSwitch(ctx, core.AutoSwitchRequest{Request: u.request()})
	if err != nil {
		u.events().Log(core.LevelWarn, "auto-switch: "+err.Error())
	}
	fyne.Do(func() {
		// The window redraws only when someone can see it; hidden to the
		// tray, the state is read again when it is shown.
		if u.hiddenToTray || !u.sh.OnScreen() {
			return
		}
		if res.Switched || u.statusOK {
			u.statusOK = false
			u.loadStatus()
		}
	})
}

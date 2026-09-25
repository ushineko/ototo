/*
Package notify sends desktop notifications over D-Bus (spec 001 D5).

The original ran notify-send; this is the same call, org.freedesktop
.Notifications.Notify on the session bus, without the subprocess. The hints
are the ones the original passed, so the notification sounds and looks the
same on the same desktop.
*/
package notify

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

// AppName is what the desktop shows as the sender.
const AppName = "ototo"

// Icons and sounds the original used.
const (
	IconAudio   = "audio-card"
	IconFailure = "dialog-error"
	SoundNew    = "message-new-instant"
)

// Notification is one message.
type Notification struct {
	Title string
	Body  string
	// Icon is a theme icon name; empty means IconAudio.
	Icon string
	// Sound is a theme sound name; empty means SoundNew.
	Sound string
	// Replaces is the id of an earlier notification to update in place, 0
	// for a new one.
	Replaces uint32
	// Timeout in milliseconds; 0 lets the desktop decide, -1 too.
	Timeout int32
}

// Notifier sends notifications. Send returns the id the desktop assigned.
type Notifier interface {
	Send(n Notification) (uint32, error)
}

// Bus is a Notifier over the session bus.
type Bus struct {
	conn *dbus.Conn
}

// Session connects to the session bus. The connection is shared with the
// rest of the process by godbus, so this is cheap to call more than once.
func Session() (*Bus, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to the session bus: %w", err)
	}
	return &Bus{conn: conn}, nil
}

// Send delivers one notification.
func (b *Bus) Send(n Notification) (uint32, error) {
	if n.Icon == "" {
		n.Icon = IconAudio
	}
	if n.Sound == "" {
		n.Sound = SoundNew
	}
	hints := map[string]dbus.Variant{
		"sound-name": dbus.MakeVariant(n.Sound),
	}
	obj := b.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	var id uint32
	err := obj.Call("org.freedesktop.Notifications.Notify", 0,
		AppName, n.Replaces, n.Icon, n.Title, n.Body, []string{}, hints, n.Timeout).Store(&id)
	if err != nil {
		return 0, fmt.Errorf("send the notification: %w", err)
	}
	return id, nil
}

// Discard is a Notifier that drops everything: for a machine with no
// notification service, and for tests.
type Discard struct{}

// Send drops the notification.
func (Discard) Send(Notification) (uint32, error) { return 0, nil }

// Recorder keeps what was sent, for tests.
type Recorder struct {
	Sent []Notification
}

// Send records the notification.
func (r *Recorder) Send(n Notification) (uint32, error) {
	r.Sent = append(r.Sent, n)
	return uint32(len(r.Sent)), nil //nolint:gosec // a test counter
}

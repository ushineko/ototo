/*
Package bluetooth reads and connects paired audio devices through BlueZ
(spec 001 R9.1), over the system bus with godbus. No bluetoothctl.

What the switcher needs is small: the paired devices that can play audio,
with their alias and whether they are connected, and Connect and Disconnect
on one of them. The alias is what names a bluez sink in the device list, so
this is also where the device model's names come from.
*/
package bluetooth

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/ushineko/ototo/internal/devices"
)

const (
	bluezName   = "org.bluez"
	deviceIface = "org.bluez.Device1"
)

// audioUUIDs are the profiles that mean a device plays or records audio, as
// the original listed them: A2DP sink and source, headset, handsfree, AADP.
var audioUUIDs = map[string]bool{
	"0000110b-0000-1000-8000-00805f9b34fb": true,
	"0000110a-0000-1000-8000-00805f9b34fb": true,
	"00001108-0000-1000-8000-00805f9b34fb": true,
	"0000111e-0000-1000-8000-00805f9b34fb": true,
	"0000110d-0000-1000-8000-00805f9b34fb": true,
}

// managed is the shape of ObjectManager.GetManagedObjects.
type managed map[dbus.ObjectPath]map[string]map[string]dbus.Variant

// Adapter is the BlueZ service on the system bus.
type Adapter struct {
	conn *dbus.Conn
}

// System connects to the system bus. It succeeds on a machine with no
// BlueZ; Devices then reports the absence.
func System() (*Adapter, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("connect to the system bus: %w", err)
	}
	return &Adapter{conn: conn}, nil
}

// Devices lists the paired devices that can play audio, by alias.
func (a *Adapter) Devices(ctx context.Context) ([]devices.Bluetooth, error) {
	objs, err := a.managedObjects(ctx)
	if err != nil {
		return nil, err
	}
	return filterAudio(objs), nil
}

func (a *Adapter) managedObjects(ctx context.Context) (managed, error) {
	var objs managed
	err := a.conn.Object(bluezName, "/").CallWithContext(ctx,
		"org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objs)
	if err != nil {
		return nil, fmt.Errorf("list the Bluetooth devices: %w", err)
	}
	return objs, nil
}

// filterAudio is the pure half of Devices.
func filterAudio(objs managed) []devices.Bluetooth {
	var out []devices.Bluetooth
	for _, ifaces := range objs {
		props, ok := ifaces[deviceIface]
		if !ok {
			continue
		}
		mac := str(props["Address"])
		if mac == "" {
			continue
		}
		icon := str(props["Icon"])
		audio := strings.HasPrefix(icon, "audio-")
		if uuids, ok := props["UUIDs"].Value().([]string); ok {
			for _, u := range uuids {
				if audioUUIDs[strings.ToLower(u)] {
					audio = true
					break
				}
			}
		}
		if !audio {
			continue
		}
		name := str(props["Alias"])
		if name == "" {
			name = str(props["Name"])
		}
		if name == "" {
			name = mac
		}
		connected, _ := props["Connected"].Value().(bool)
		out = append(out, devices.Bluetooth{MAC: strings.ToUpper(mac), Name: name, Connected: connected})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func str(v dbus.Variant) string {
	s, _ := v.Value().(string)
	return s
}

// Connect asks BlueZ to connect the device with this address. It returns
// when BlueZ answers, which is before the sink appears; the caller waits
// for that.
func (a *Adapter) Connect(ctx context.Context, mac string) error {
	return a.call(ctx, mac, "Connect")
}

// Disconnect asks BlueZ to disconnect the device.
func (a *Adapter) Disconnect(ctx context.Context, mac string) error {
	return a.call(ctx, mac, "Disconnect")
}

// call finds the device's object by address, on whichever adapter holds it,
// and calls one Device1 method. The original assumed hci0; a machine with a
// second adapter breaks that.
func (a *Adapter) call(ctx context.Context, mac, method string) error {
	objs, err := a.managedObjects(ctx)
	if err != nil {
		return err
	}
	path, ok := pathFor(objs, mac)
	if !ok {
		return fmt.Errorf("no paired Bluetooth device has the address %s", mac)
	}
	if err := a.conn.Object(bluezName, path).CallWithContext(ctx, deviceIface+"."+method, 0).Err; err != nil {
		return fmt.Errorf("%s %s: %w", strings.ToLower(method), mac, err)
	}
	return nil
}

func pathFor(objs managed, mac string) (dbus.ObjectPath, bool) {
	for path, ifaces := range objs {
		if props, ok := ifaces[deviceIface]; ok && strings.EqualFold(str(props["Address"]), mac) {
			return path, true
		}
	}
	return "", false
}

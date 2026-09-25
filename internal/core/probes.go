package core

import (
	"context"
	"time"

	"github.com/ushineko/ototo/internal/bluetooth"
	"github.com/ushineko/ototo/internal/devices"
	"github.com/ushineko/ototo/internal/headset"
)

/*
Probes are the two readers outside the sound server that the device model
takes as inputs: the Bluetooth adapter and the headset tool (R9.1, R9.2).

They are a value on the request so that a test supplies its own, and so
that a machine without an adapter or the tool is a machine that reads as
"no paired devices" and "headset off" rather than one that fails.
*/
type Probes struct {
	// Bluetooth lists the paired audio devices; nil means none.
	Bluetooth func(ctx context.Context) []devices.Bluetooth
	// Connect asks the adapter to connect one device; nil cannot.
	Connect func(ctx context.Context, mac string) error
	// Disconnect asks the adapter to disconnect one device; nil cannot.
	Disconnect func(ctx context.Context, mac string) error
	// Headset reads the battery; nil means not detected.
	Headset func(ctx context.Context) devices.Headset
	// SetIdle sets the headset's idle timeout; nil cannot.
	SetIdle func(ctx context.Context, minutes int) error
}

// probeTimeout bounds one read of the adapter: a BlueZ that hangs must not
// stall a tick.
const probeTimeout = 3 * time.Second

// DefaultProbes reads the machine: BlueZ over the system bus when it
// answers, headsetcontrol when it is installed.
func DefaultProbes() Probes {
	p := Probes{}
	if adapter, err := bluetooth.System(); err == nil {
		p.Bluetooth = func(ctx context.Context) []devices.Bluetooth {
			ctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			list, err := adapter.Devices(ctx)
			if err != nil {
				return nil
			}
			return list
		}
		p.Connect = adapter.Connect
		p.Disconnect = adapter.Disconnect
	}
	if headset.Available() {
		tool := headset.New()
		p.Headset = tool.Battery
		p.SetIdle = tool.SetIdle
	}
	return p
}

// demoProbes are the file's paired devices and headset (audio.DemoFile).
func demoProbes(server string) Probes {
	d, err := demoServer(server)
	if err != nil {
		return Probes{}
	}
	f := d.File()
	var bt []devices.Bluetooth
	for _, b := range f.Bluetooth {
		bt = append(bt, devices.Bluetooth{MAC: b.MAC, Name: b.Name, Connected: b.Connected})
	}
	return Probes{
		Bluetooth: func(context.Context) []devices.Bluetooth { return bt },
		Connect:   func(context.Context, string) error { return nil },
		Disconnect: func(context.Context, string) error {
			return nil
		},
		Headset: func(context.Context) devices.Headset {
			return devices.Headset{Detected: f.Headset.Detected, Battery: f.Headset.Battery}
		},
		SetIdle: func(context.Context, int) error { return nil },
	}
}

func (p Probes) bluetooth(ctx context.Context) []devices.Bluetooth {
	if p.Bluetooth == nil {
		return nil
	}
	return p.Bluetooth(ctx)
}

func (p Probes) headset(ctx context.Context) devices.Headset {
	if p.Headset == nil {
		return devices.Headset{}
	}
	return p.Headset(ctx)
}

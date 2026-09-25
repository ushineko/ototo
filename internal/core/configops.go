package core

import (
	"context"
	"fmt"

	"github.com/ushineko/ototo/internal/config"
	"github.com/ushineko/ototo/internal/devices"
)

// SetAutoSwitchRequest turns the auto-switch on or off.
type SetAutoSwitchRequest struct {
	Request
	Enabled bool
}

// SetAutoSwitch writes auto_switch and returns the settings as saved.
func SetAutoSwitch(_ context.Context, req SetAutoSwitchRequest) (config.Config, error) {
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return cfg, err
	}
	cfg.AutoSwitch = req.Enabled
	if err := config.Save(path, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// DisconnectRequest names a Bluetooth device to disconnect.
type DisconnectRequest struct {
	Request
	// Target is resolved as Switch resolves it.
	Target string
}

// Disconnect asks the adapter to drop a connected Bluetooth device, which
// makes its sink go away and lets the auto-switch move on.
func (s *Switcher) Disconnect(ctx context.Context, req DisconnectRequest) (devices.Device, error) {
	srv, err := s.connect(req.Server)
	if err != nil {
		return devices.Device{}, err
	}
	defer func() { _ = srv.Close() }()
	cfg, _, err := config.Load(req.ConfigPath)
	if err != nil {
		return devices.Device{}, err
	}
	probes := req.probes()
	list, err := s.list(srv, devices.Inputs{
		Priority: cfg.DevicePriority, Bluetooth: probes.bluetooth(ctx), Headset: probes.headset(ctx),
	})
	if err != nil {
		return devices.Device{}, err
	}
	dev, ok := Resolve(list, req.Target)
	if !ok {
		return devices.Device{}, fmt.Errorf("%w %q", ErrNotFound, req.Target)
	}
	if dev.MAC == "" {
		return dev, fmt.Errorf("%s is not a Bluetooth device", dev.Name)
	}
	if probes.Disconnect == nil {
		return dev, fmt.Errorf("there is no Bluetooth adapter to disconnect %s with", dev.Name)
	}
	if err := probes.Disconnect(ctx, dev.MAC); err != nil {
		return dev, err
	}
	req.Events.logf(LevelInfo, "disconnected %s", dev.Name)
	return dev, nil
}

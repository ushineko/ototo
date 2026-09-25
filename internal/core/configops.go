package core

import (
	"context"
	"errors"
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

// ConnectRequest names a Bluetooth device to connect, without switching.
type ConnectRequest struct {
	Request
	// Target is resolved as Switch resolves it.
	Target string
}

/*
Connect asks the adapter to connect a paired device and waits for its sink
to appear, and does not switch to it: with the auto-switch on, the order
decides whether it plays; off, Switch to does. It is the half of R5.5's
offline branch that a person wants on its own, to bring a headset up before
they need it.
*/
func (s *Switcher) Connect(ctx context.Context, req ConnectRequest) (devices.Device, error) {
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
	in := devices.Inputs{
		Priority: cfg.DevicePriority, Bluetooth: probes.bluetooth(ctx), Headset: probes.headset(ctx),
	}
	list, err := s.list(srv, in)
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
	if dev.Online {
		return dev, nil
	}
	return s.connectAndWait(ctx, srv, in, probes, dev, req.Events)
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

// SetPriorityRequest writes the auto-switch order.
type SetPriorityRequest struct {
	Request
	// Order is the full list of ids, highest first.
	Order []string
}

// SetPriority writes device_priority as given.
func SetPriority(_ context.Context, req SetPriorityRequest) (config.Config, error) {
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return cfg, err
	}
	cfg.DevicePriority = append([]string{}, req.Order...)
	if err := config.Save(path, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Moved returns order with id moved by delta places (-1 up, +1 down), or
// order unchanged when the move is impossible. An id not yet in the order
// is appended first, so a device the user ranks for the first time joins
// the list at the end and then moves.
func Moved(order []string, id string, delta int) []string {
	out := append([]string{}, order...)
	at := -1
	for i, o := range out {
		if o == id {
			at = i
			break
		}
	}
	if at < 0 {
		out = append(out, id)
		at = len(out) - 1
	}
	to := at + delta
	if to < 0 || to >= len(out) {
		return out
	}
	out[at], out[to] = out[to], out[at]
	return out
}

// SetMicLinkRequest pins an input to an output, or lets it be matched, or
// leaves it alone.
type SetMicLinkRequest struct {
	Request
	// Device is the output's priority id.
	Device string
	// Link is config.MicAuto, config.MicDefault or a source name.
	Link string
}

// SetMicLink writes mic_links[Device]. Auto is the default, so it is
// stored by removing the key, as the original did.
func SetMicLink(_ context.Context, req SetMicLinkRequest) (config.Config, error) {
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return cfg, err
	}
	if req.Link == "" || req.Link == config.MicAuto {
		delete(cfg.MicLinks, req.Device)
	} else {
		cfg.MicLinks[req.Device] = req.Link
	}
	if err := config.Save(path, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SetSwitchesRequest changes the on/off settings. A nil field is left as it
// is, so one control writes one key.
type SetSwitchesRequest struct {
	Request
	OSDEnabled          *bool
	OSDTextSize         *int
	OSDFont             *string
	SwitchNotifications *bool
	SwitchInOSD         *bool
	MoveStreams         *bool
	// LoopbackSource picks the line-in source; "" means the first found.
	LoopbackSource *string
}

// OSD text size bounds, in points.
const (
	OSDTextSizeMin = 8
	OSDTextSizeMax = 96
)

// OSDTextSizes are the sizes the window offers, in points.
var OSDTextSizes = []int{16, 20, 24, 28, 32, 40, 48, 64}

// SetSwitches writes the switches that are set.
func SetSwitches(_ context.Context, req SetSwitchesRequest) (config.Config, error) {
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return cfg, err
	}
	if req.OSDEnabled != nil {
		cfg.OSDEnabled = *req.OSDEnabled
	}
	if req.OSDTextSize != nil {
		if *req.OSDTextSize < OSDTextSizeMin || *req.OSDTextSize > OSDTextSizeMax {
			return cfg, fmt.Errorf("the indicator text size is %d to %d points, not %d", OSDTextSizeMin, OSDTextSizeMax, *req.OSDTextSize)
		}
		cfg.OSDTextSize = *req.OSDTextSize
	}
	if req.OSDFont != nil {
		cfg.OSDFont = *req.OSDFont
	}
	if req.SwitchNotifications != nil {
		cfg.SwitchNotifications = *req.SwitchNotifications
	}
	if req.SwitchInOSD != nil {
		cfg.SwitchInOSD = *req.SwitchInOSD
	}
	if req.MoveStreams != nil {
		cfg.MoveStreams = *req.MoveStreams
	}
	if req.LoopbackSource != nil {
		cfg.LoopbackSource = *req.LoopbackSource
	}
	if err := config.Save(path, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SetHeadsetIdleRequest sets the Arctis idle timeout.
type SetHeadsetIdleRequest struct {
	Request
	// Minutes is 0 for never, else 1..90.
	Minutes int
}

// SetHeadsetIdle writes arctis_idle_minutes and applies it to the headset
// (R9.2). The setting is written first: a headset that is off right now
// still gets the value the next time it is on and the setting is applied.
func SetHeadsetIdle(ctx context.Context, req SetHeadsetIdleRequest) (config.Config, error) {
	if req.Minutes < 0 || req.Minutes > 90 {
		return config.Config{}, fmt.Errorf("the idle timeout is 0 (never) or 1 to 90 minutes, not %d", req.Minutes)
	}
	cfg, path, err := config.Load(req.ConfigPath)
	if err != nil {
		return cfg, err
	}
	cfg.ArctisIdleMinutes = req.Minutes
	if err := config.Save(path, cfg); err != nil {
		return cfg, err
	}
	probes := req.probes()
	if probes.SetIdle == nil {
		return cfg, errors.New("headsetcontrol is not installed, so the timeout was saved but not applied")
	}
	if err := probes.SetIdle(ctx, req.Minutes); err != nil {
		return cfg, fmt.Errorf("the timeout was saved but not applied: %w", err)
	}
	req.Events.logf(LevelInfo, "headset idle timeout set to %d minutes", req.Minutes)
	return cfg, nil
}

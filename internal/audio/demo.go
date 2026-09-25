package audio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

/*
A demo server, for the screenshot harness and for a test of the whole
window: --server demo:<file> makes every operation run over the devices the
file describes rather than the machine's, so a picture in the README can
show invented devices with documentation-range addresses and nothing of
anyone's.

It is the same interface the real client has, kept in memory: a switch
changes the default, a volume step changes the level, and the window reads
back what it wrote. What it cannot do is play sound, and it does not try.
*/

// DemoPrefix marks a --server value as a demo file.
const DemoPrefix = "demo:"

// DemoFile is the shape of the file.
type DemoFile struct {
	DefaultSink   string       `json:"default_sink"`
	DefaultSource string       `json:"default_source"`
	Sinks         []DemoDevice `json:"sinks"`
	Sources       []DemoDevice `json:"sources"`
	// Bluetooth and Headset feed the device model's probes, so the list
	// shows paired devices and a headset's battery without an adapter or
	// the tool.
	Bluetooth []DemoBluetooth `json:"bluetooth"`
	Headset   DemoHeadset     `json:"headset"`
}

// DemoDevice is one sink or source in the file.
type DemoDevice struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Properties  map[string]string `json:"properties"`
	Ports       []DemoPort        `json:"ports"`
	ActivePort  string            `json:"active_port"`
	Volume      int               `json:"volume"`
	Mute        bool              `json:"mute"`
}

// DemoPort is one port; Available is "yes", "no" or "" for unknown.
type DemoPort struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Available   string `json:"available"`
}

// DemoBluetooth is one paired device.
type DemoBluetooth struct {
	MAC       string `json:"mac"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

// DemoHeadset is the headset tool's answer.
type DemoHeadset struct {
	Detected bool   `json:"detected"`
	Battery  string `json:"battery"`
}

// Demo is the in-memory server.
type Demo struct {
	mu   sync.Mutex
	file DemoFile
}

// IsDemo reports whether a --server value names a demo file.
func IsDemo(server string) bool { return strings.HasPrefix(server, DemoPrefix) }

// LoadDemo reads the file a demo server value names.
func LoadDemo(server string) (*Demo, error) {
	path := strings.TrimPrefix(server, DemoPrefix)
	if path == "" {
		return nil, errors.New("demo: no file named")
	}
	raw, err := os.ReadFile(path) //nolint:gosec // the path the person gave on the command line
	if err != nil {
		return nil, fmt.Errorf("demo: read %s: %w", path, err)
	}
	var f DemoFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("demo: parse %s: %w", path, err)
	}
	return &Demo{file: f}, nil
}

// File is the file as loaded, for the probes.
func (d *Demo) File() DemoFile {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.file
}

// Server answers as the real one does.
func (d *Demo) Server() (Server, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Server{Name: "demo server", Version: "0", DefaultSink: d.file.DefaultSink, DefaultSource: d.file.DefaultSource}, nil
}

func (d *Demo) devices(list []DemoDevice) []Device {
	out := make([]Device, 0, len(list))
	for i, s := range list {
		dev := Device{
			Index:         uint32(i), //nolint:gosec // a short list
			Name:          s.Name,
			Description:   s.Description,
			Properties:    s.Properties,
			ActivePort:    s.ActivePort,
			Mute:          s.Mute,
			VolumePercent: s.Volume,
		}
		if dev.Properties == nil {
			dev.Properties = map[string]string{}
		}
		for _, p := range s.Ports {
			avail := AvailabilityUnknown
			switch p.Available {
			case "yes":
				avail = AvailabilityYes
			case "no":
				avail = AvailabilityNo
			}
			dev.Ports = append(dev.Ports, Port{Name: p.Name, Description: p.Description, Availability: avail})
		}
		out = append(out, dev)
	}
	return out
}

// Sinks lists the file's sinks.
func (d *Demo) Sinks() ([]Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.devices(d.file.Sinks), nil
}

// Sources lists the file's sources.
func (d *Demo) Sources() ([]Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.devices(d.file.Sources), nil
}

// SetDefaultSink sets the default, which must be a sink in the file.
func (d *Demo) SetDefaultSink(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sink(name) < 0 {
		return fmt.Errorf("demo: no sink %s", name)
	}
	d.file.DefaultSink = name
	return nil
}

// SetDefaultSource sets the default input; any name is accepted, as the
// real server accepts a pinned input that appears later.
func (d *Demo) SetDefaultSource(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.file.DefaultSource = name
	return nil
}

// MoveSinkInputs reports two streams moved and none refused.
func (d *Demo) MoveSinkInputs(string) (int, int, error) { return 2, 0, nil }

func (d *Demo) sink(name string) int {
	for i, s := range d.file.Sinks {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// Volume reads a sink's level and mute.
func (d *Demo) Volume(sink string) (int, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.sink(sink)
	if i < 0 {
		return 0, false, fmt.Errorf("demo: no sink %s", sink)
	}
	return d.file.Sinks[i].Volume, d.file.Sinks[i].Mute, nil
}

// SetVolume sets a sink's level, clamped as the real one clamps.
func (d *Demo) SetVolume(sink string, percent int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.sink(sink)
	if i < 0 {
		return 0, fmt.Errorf("demo: no sink %s", sink)
	}
	d.file.Sinks[i].Volume = clamp(percent)
	return d.file.Sinks[i].Volume, nil
}

// AdjustVolume steps a sink's level.
func (d *Demo) AdjustVolume(sink string, delta int) (int, error) {
	current, _, err := d.Volume(sink)
	if err != nil {
		return 0, err
	}
	return d.SetVolume(sink, current+delta)
}

// SetMute mutes or unmutes a sink.
func (d *Demo) SetMute(sink string, mute bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.sink(sink)
	if i < 0 {
		return fmt.Errorf("demo: no sink %s", sink)
	}
	d.file.Sinks[i].Mute = mute
	return nil
}

// Close does nothing; the state lives for the process.
func (d *Demo) Close() error { return nil }

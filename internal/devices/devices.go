/*
Package devices is the device model (spec 001 R4): what the sound server's
sinks, the settings' priority list and the Bluetooth adapter's paired devices
become when a person looks at them as one list.

It is a pure function of its inputs. Nothing here reaches a server; the
callers read the sinks, the config and the Bluetooth cache and hand them in,
which is what lets the naming and the merge be tested over synthetic property
sets without a machine that has the devices.

The rules are the original program's, function for function, because the
priority ids are what a user's config.json already holds and a rename would
orphan every entry in it.
*/
package devices

import (
	"regexp"
	"strings"

	"github.com/ushineko/ototo/internal/audio"
)

// JamesDSPSink is the virtual sink JamesDSP registers. It never appears in
// the list: the program routes through it, and a person choosing it as an
// output would route effects to nothing.
const JamesDSPSink = "jamesdsp_sink"

// BluetoothPrefix marks a priority id keyed by address rather than sink name.
const BluetoothPrefix = "bt:"

// macPattern matches the address inside a bluez sink name, with either
// separator the server uses.
var macPattern = regexp.MustCompile(`(?i)([0-9A-F]{2}[:_][0-9A-F]{2}[:_][0-9A-F]{2}[:_][0-9A-F]{2}[:_][0-9A-F]{2}[:_][0-9A-F]{2})`)

// Bluetooth is one paired device the adapter knows, audio-capable.
type Bluetooth struct {
	MAC       string // upper case, colon separated
	Name      string // the alias, else the name, else the address
	Connected bool
}

// Headset is what headsetcontrol reported, for a sink whose name says it is
// a SteelSeries Arctis. Detected false means the tool answered nothing, which
// the original treats as the headset being off.
type Headset struct {
	Detected bool
	Battery  string // "87%"
}

// Inputs is everything the list is made from.
type Inputs struct {
	Sinks       []audio.Device
	DefaultSink string
	// Priority is config.device_priority: ids, highest first.
	Priority []string
	// Bluetooth is the adapter's paired audio devices; nil when there is no
	// adapter or the cache has not been read.
	Bluetooth []Bluetooth
	Headset   Headset
}

// Device is one row of the list.
type Device struct {
	// ID is the priority id: BluetoothPrefix and the address for a Bluetooth
	// device, the sink name for anything else.
	ID string
	// Name is what a person calls it, with the port and the state appended
	// as the original did: "Arctis Nova Pro Wireless [87%]",
	// "Speakers - Line Out", "AirPods [Disconnected]".
	Name string
	// Sink is the sink name; empty for a device with no sink right now.
	Sink string
	// Online means a sink exists. Connected means it can play: a sink whose
	// port reports nothing plugged in is online and not connected.
	Online    bool
	Connected bool
	Default   bool
	MAC       string // set for a Bluetooth device
	Volume    int
	Mute      bool
	// Properties are the sink's, kept for the microphone association (R7).
	Properties map[string]string
}

// PriorityID is R4.1: the address when the sink name carries one, else the
// sink name.
func PriorityID(sinkName string) string {
	if mac := macIn(sinkName); mac != "" {
		return BluetoothPrefix + mac
	}
	return sinkName
}

func macIn(s string) string {
	m := macPattern.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(m[1], "_", ":"))
}

// DisplayName is R4.2 without the suffixes: the alias for a Bluetooth sink,
// then vendor and product, then the description, then the sink name.
func DisplayName(sink audio.Device, aliases map[string]string) string {
	props := sink.Properties
	name := ""
	if strings.Contains(sink.Name, "bluez") || strings.Contains(props["device.api"], "bluez") {
		if mac := macIn(sink.Name); mac != "" {
			name = aliases[mac]
		}
		if name == "" {
			name = props["bluez.alias"]
		}
		if name == "" {
			name = props["device.alias"]
		}
	}
	if name == "" {
		vendor := props["device.vendor.name"]
		product := props["device.product.name"]
		if product == "" {
			product = props["device.model"]
		}
		if vendor != "" && product != "" {
			name = vendor + " " + product
		}
	}
	if name == "" {
		name = props["device.description"]
	}
	name = strings.TrimSpace(strings.ReplaceAll(name, "(null)", ""))
	if name == "" {
		name = sink.Name
	}
	return name
}

// isHeadset says whether a display name is the SteelSeries headset the
// original knows how to ask about.
func isHeadset(name string) bool {
	return strings.Contains(name, "Arctis Nova") || strings.Contains(name, "SteelSeries")
}

// describe builds one row from a live sink: the name with its suffixes, and
// whether it can play.
func describe(sink audio.Device, in Inputs, aliases map[string]string) Device {
	name := DisplayName(sink, aliases)
	connected := true

	if isHeadset(name) {
		if in.Headset.Detected {
			name += " [" + in.Headset.Battery + "]"
		} else {
			name += " [Disconnected]"
			connected = false
		}
	}

	if sink.ActivePort != "" {
		for _, p := range sink.Ports {
			if p.Name != sink.ActivePort {
				continue
			}
			desc := p.Description
			if desc == "" {
				desc = p.Name
			}
			if desc != "Analog Output" {
				name += " - " + desc
			}
			if p.Availability == audio.AvailabilityNo {
				if connected {
					name += " [Disconnected]"
				}
				connected = false
			}
			break
		}
	}

	return Device{
		ID:         PriorityID(sink.Name),
		Name:       name,
		Sink:       sink.Name,
		Online:     true,
		Connected:  connected,
		Default:    sink.Name == in.DefaultSink,
		MAC:        macIn(sink.Name),
		Volume:     sink.VolumePercent,
		Mute:       sink.Mute,
		Properties: sink.Properties,
	}
}

/*
List is R4.3 and R4.4: one list, in this order.

 1. Every id in Priority, in that order: the live sink for it, or a
    remembered device with no sink, marked disconnected.
 2. Every live sink not yet listed, in the server's order.
 3. Every paired Bluetooth audio device not yet listed, so that a headset
    paired but never chosen can be chosen.

The JamesDSP sink never appears, and an id is listed once.
*/
func List(in Inputs) []Device {
	aliases := make(map[string]string, len(in.Bluetooth))
	for _, b := range in.Bluetooth {
		aliases[b.MAC] = b.Name
	}
	online := make(map[string]Device, len(in.Sinks))
	order := make([]string, 0, len(in.Sinks))
	for _, s := range in.Sinks {
		if s.Name == JamesDSPSink {
			continue
		}
		d := describe(s, in, aliases)
		if _, dup := online[d.ID]; dup {
			continue
		}
		online[d.ID] = d
		order = append(order, d.ID)
	}

	var out []Device
	seen := map[string]bool{}
	add := func(d Device) {
		if seen[d.ID] {
			return
		}
		seen[d.ID] = true
		out = append(out, d)
	}

	for _, id := range in.Priority {
		if strings.Contains(id, JamesDSPSink) {
			continue
		}
		if d, ok := online[id]; ok {
			add(d)
			continue
		}
		add(offline(id, aliases))
	}
	for _, id := range order {
		add(online[id])
	}
	for _, b := range in.Bluetooth {
		add(offline(BluetoothPrefix+b.MAC, aliases))
	}
	return out
}

// offline is a remembered device with no sink: named from the adapter when
// it is a Bluetooth id, else by its id, and marked disconnected.
func offline(id string, aliases map[string]string) Device {
	d := Device{ID: id, Name: id}
	if mac, ok := strings.CutPrefix(id, BluetoothPrefix); ok {
		d.MAC = mac
		if alias := aliases[mac]; alias != "" {
			d.Name = alias
		}
	}
	d.Name += " [Disconnected]"
	return d
}

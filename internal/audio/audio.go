/*
Package audio talks to the sound server over the PulseAudio native protocol
(spec 001 R3).

PipeWire serves that protocol on the same socket PulseAudio did, so one client
covers both, and it is the protocol `pactl` itself speaks: everything the PyQt6
program shelled out to pactl for -- sinks, sources, the defaults, volume, mute,
stream moves and the change subscription -- is a request here, in Go, with no
subprocess and no CGO. The graph rewiring that JamesDSP needs is a different
protocol (PipeWire's own) and stays behind pw-link; see package graph, when it
lands.

This file holds the connection and the read-only queries the status operation
needs. The rest of the surface arrives with the specs that use it.
*/
package audio

import (
	"fmt"
	"net"
	"sort"

	"github.com/jfreymuth/pulse/proto"
)

// volumeNorm is PA_VOLUME_NORM: the value that is 100%.
const volumeNorm = 0x10000

// Port availability as the server reports it.
const (
	AvailabilityUnknown = 0
	AvailabilityNo      = 1
	AvailabilityYes     = 2
)

// Server is what the sound server says about itself.
type Server struct {
	Name          string
	Version       string
	DefaultSink   string
	DefaultSource string
}

// Port is one physical connector of a sink or source, and whether something
// is plugged into it. Jack detection reports through Availability.
type Port struct {
	Name         string
	Description  string
	Availability int
}

// Device is one sink or source as the server reports it.
type Device struct {
	Index       uint32
	Name        string
	Description string
	Properties  map[string]string
	Ports       []Port
	ActivePort  string
	Mute        bool
	// VolumePercent is the loudest channel, as a percentage of normal.
	VolumePercent int
}

// Connected reports whether the active port has something plugged into it. A
// device with no ports, or whose port reports nothing, counts as connected:
// only a port that says "no" is disconnected.
func (d Device) Connected() bool {
	for _, p := range d.Ports {
		if p.Name == d.ActivePort {
			return p.Availability != AvailabilityNo
		}
	}
	return true
}

// Client is one connection to the server.
type Client struct {
	c    *proto.Client
	conn net.Conn
}

// Connect opens the server named by server, or the default one when it is
// empty: $PULSE_SERVER, else $XDG_RUNTIME_DIR/pulse/native.
func Connect(server string) (*Client, error) {
	c, conn, err := proto.Connect(server)
	if err != nil {
		return nil, fmt.Errorf("connect to the sound server: %w", err)
	}
	props := proto.PropList{
		"application.name": proto.PropListString("ototo"),
		"application.id":   proto.PropListString("io.ushineko.ototo"),
	}
	if err := c.Request(&proto.SetClientName{Props: props}, &proto.SetClientNameReply{}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("introduce the client to the sound server: %w", err)
	}
	return &Client{c: c, conn: conn}, nil
}

// Close ends the connection.
func (c *Client) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("close the sound server connection: %w", err)
	}
	return nil
}

// Server asks the server what it is and what its defaults are.
func (c *Client) Server() (Server, error) {
	var r proto.GetServerInfoReply
	if err := c.c.Request(&proto.GetServerInfo{}, &r); err != nil {
		return Server{}, fmt.Errorf("read the server info: %w", err)
	}
	return Server{
		Name:          r.PackageName,
		Version:       r.PackageVersion,
		DefaultSink:   r.DefaultSinkName,
		DefaultSource: r.DefaultSourceName,
	}, nil
}

// Sinks lists every output, in index order.
func (c *Client) Sinks() ([]Device, error) {
	var r proto.GetSinkInfoListReply
	if err := c.c.Request(&proto.GetSinkInfoList{}, &r); err != nil {
		return nil, fmt.Errorf("list the outputs: %w", err)
	}
	out := make([]Device, 0, len(r))
	for _, s := range r {
		d := Device{
			Index:         s.SinkIndex,
			Name:          s.SinkName,
			Description:   s.Device,
			Properties:    properties(s.Properties),
			ActivePort:    s.ActivePortName,
			Mute:          s.Mute,
			VolumePercent: Percent(s.ChannelVolumes),
		}
		for _, p := range s.Ports {
			d.Ports = append(d.Ports, Port{Name: p.Name, Description: p.Description, Availability: int(p.Available)})
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

// Sources lists every input, in index order. Monitors of outputs are left out:
// they are not something a person plugs a microphone into.
func (c *Client) Sources() ([]Device, error) {
	var r proto.GetSourceInfoListReply
	if err := c.c.Request(&proto.GetSourceInfoList{}, &r); err != nil {
		return nil, fmt.Errorf("list the inputs: %w", err)
	}
	out := make([]Device, 0, len(r))
	for _, s := range r {
		props := properties(s.Properties)
		if props["device.class"] == "monitor" {
			continue
		}
		d := Device{
			Index:         s.SourceIndex,
			Name:          s.SourceName,
			Description:   s.Device,
			Properties:    props,
			ActivePort:    s.ActivePortName,
			Mute:          s.Mute,
			VolumePercent: Percent(s.ChannelVolumes),
		}
		for _, p := range s.Ports {
			d.Ports = append(d.Ports, Port{Name: p.Name, Description: p.Description, Availability: int(p.Available)})
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

// Percent is the loudest channel as a whole percentage of normal volume,
// rounded the way pactl rounds it.
func Percent(v proto.ChannelVolumes) int {
	var loudest proto.Volume
	for _, ch := range v {
		if ch > loudest {
			loudest = ch
		}
	}
	return int((uint64(loudest)*100 + volumeNorm/2) / volumeNorm)
}

func properties(p proto.PropList) map[string]string {
	out := make(map[string]string, len(p))
	for k, v := range p {
		out[k] = v.String()
	}
	return out
}

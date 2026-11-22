package system

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
)

// CatalogSource is the slice of the kit's client the catalogue uses.
type CatalogSource interface {
	Interfaces(ctx context.Context) ([]litesource.Interface, error)
	Devices(ctx context.Context, iface string) ([]litesource.DeviceDescription, error)
	Names(ctx context.Context) (*litesource.Names, error)
	ParamsetsFor(ctx context.Context, iface string, addresses []string, paramset string) (keys map[string]string, sets litesource.ParamsetCache, errs map[string]string, err error)
}

// Channel is one channel of a device, as the editor's picker lists it.
type Channel struct {
	// Address is the channel's, "ADDRESS:CH".
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
	// Type is the channel's type (SHUTTER_CONTACT, SWITCH_VIRTUAL_RECEIVER …).
	Type string `json:"type"`
	// Rooms and Functions are the names of the rooms and functions the channel is in.
	Rooms     []string `json:"rooms,omitempty"`
	Functions []string `json:"functions,omitempty"`
}

// Device is one device with its channels.
type Device struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Name      string `json:"name,omitempty"`
	// Type is the device's type (HmIP-SWDO …).
	Type     string    `json:"type"`
	Channels []Channel `json:"channels"`
}

// Param is one datapoint of a channel: what a condition can look at and an activity can write.
type Param struct {
	ID string `json:"id"`
	// Type: BOOL, ACTION, INTEGER, FLOAT, ENUM, STRING.
	Type string `json:"type"`
	// Read: the value can be asked for or is reported (a condition can look at it); Write: it
	// can be set (an activity can write it).
	Read  bool `json:"read"`
	Write bool `json:"write"`
	// Min and Max bound a number; Unit is its unit; Values are an ENUM's entries by index.
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	Values []string `json:"values,omitempty"`
}

// Catalog answers what the system has: the devices with their channels, names and rooms, and
// the datapoints of a channel. Nothing is kept: the metadata and the paramset descriptions are
// the system's, which caches them itself.
type Catalog struct {
	Lite CatalogSource
	// Own is the addon's own interface, left out ("Logic").
	Own string
}

// Devices lists every device of every interface but the addon's own, with the names and rooms
// in lang, sorted by name. An interface that does not answer is left out; channel 0 (the
// device's maintenance channel) is listed like the others.
func (c *Catalog) Devices(ctx context.Context, lang string) ([]Device, error) {
	ifs, err := c.Lite.Interfaces(ctx)
	if err != nil {
		return nil, err
	}
	// without the metadata the devices are listed by their addresses
	names, _ := c.Lite.Names(ctx)
	out := []Device{}
	for _, i := range ifs {
		if i.Name == c.Own {
			continue
		}
		descs, err := c.Lite.Devices(ctx, i.Name)
		if err != nil {
			continue
		}
		byAddr := map[string]*Device{}
		var order []string
		for _, d := range descs {
			if d.Parent != "" || strings.Contains(d.Address, ":") {
				continue
			}
			byAddr[d.Address] = &Device{Interface: i.Name, Address: d.Address, Name: names.Name(i.Name, d.Address), Type: d.Type, Channels: []Channel{}}
			order = append(order, d.Address)
		}
		for _, d := range descs {
			parent, _, isChannel := strings.Cut(d.Address, ":")
			dev := byAddr[parent]
			if !isChannel || dev == nil {
				continue
			}
			dev.Channels = append(dev.Channels, Channel{Address: d.Address, Name: names.Name(i.Name, d.Address), Type: d.Type,
				Rooms: names.RoomNames(i.Name, d.Address, lang), Functions: names.FunctionNames(i.Name, d.Address, lang)})
		}
		for _, a := range order {
			dev := byAddr[a]
			sort.SliceStable(dev.Channels, func(x, y int) bool {
				return channelIndex(dev.Channels[x].Address) < channelIndex(dev.Channels[y].Address)
			})
			out = append(out, *dev)
		}
	}
	sort.SliceStable(out, func(x, y int) bool {
		a, b := strings.ToLower(out[x].Name), strings.ToLower(out[y].Name)
		if (a == "") != (b == "") {
			return a != "" // the named ones first
		}
		if a != b {
			return a < b
		}
		return out[x].Address < out[y].Address
	})
	return out, nil
}

func channelIndex(address string) int {
	_, ch, _ := strings.Cut(address, ":")
	n := 0
	for _, r := range ch {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// Params answers the datapoints (the VALUES paramset's description) of the given channels,
// each "Interface/ADDRESS:CH", sorted by id. A channel the system does not know is left out.
func (c *Catalog) Params(ctx context.Context, channels []string) (map[string][]Param, error) {
	byIface := map[string][]string{}
	for _, ch := range channels {
		iface, addr, ok := strings.Cut(ch, "/")
		if !ok || iface == c.Own {
			continue
		}
		byIface[iface] = append(byIface[iface], addr)
	}
	out := map[string][]Param{}
	var firstErr error
	for iface, addrs := range byIface {
		keys, sets, _, err := c.Lite.ParamsetsFor(ctx, iface, addrs, "VALUES")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, addr := range addrs {
			set, ok := sets[keys[addr]]
			if !ok {
				continue
			}
			params := make([]Param, 0, len(set))
			for id, d := range set {
				params = append(params, paramOf(id, d))
			}
			sort.Slice(params, func(x, y int) bool { return params[x].ID < params[y].ID })
			out[iface+"/"+addr] = params
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// number reads a description's MIN or MAX when it is a number (an ENUM's may be a string).
func number(raw json.RawMessage) *float64 {
	var f float64
	if len(raw) == 0 || json.Unmarshal(raw, &f) != nil {
		return nil
	}
	return &f
}

package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
)

const (
	topologyPath = "/ZoneGroupTopology/Control"
	topologyNS   = "urn:schemas-upnp-org:service:ZoneGroupTopology:1"
)

// Group is a set of rooms that play together. All playback commands must go
// to the Coordinator. Bonded devices (home-theater surrounds, stereo-pair
// partners) are not listed in Members; they are part of their room's speaker.
type Group struct {
	Name        string // e.g. "Living Room" or "Kitchen + Globine"
	Coordinator Speaker
	Members     []Speaker // visible rooms in the group, including the coordinator
}

// Matches reports whether want names this group, one of its rooms, or the IP
// of one of its members (case-insensitive for names).
func (g Group) Matches(want string) bool {
	if strings.EqualFold(g.Name, want) {
		return true
	}
	for _, m := range g.Members {
		if strings.EqualFold(m.Room, want) || m.IP == want {
			return true
		}
	}
	return g.Coordinator.IP == want
}

// MemberFor returns the member room named by want (room name or IP), falling
// back to the coordinator. Use it for per-room settings such as volume.
func (g Group) MemberFor(want string) Speaker {
	for _, m := range g.Members {
		if strings.EqualFold(m.Room, want) || m.IP == want {
			return m
		}
	}
	return g.Coordinator
}

// Topology asks any speaker for the household's zone groups.
func (s Speaker) Topology(ctx context.Context) ([]Group, error) {
	body, err := s.call(ctx, topologyPath, topologyNS, "GetZoneGroupState")
	if err != nil {
		return nil, err
	}
	var state struct {
		Groups []struct {
			Coordinator string `xml:"Coordinator,attr"`
			Members     []struct {
				UUID      string `xml:"UUID,attr"`
				Location  string `xml:"Location,attr"`
				ZoneName  string `xml:"ZoneName,attr"`
				Invisible string `xml:"Invisible,attr"`
			} `xml:"ZoneGroupMember"`
		} `xml:"ZoneGroups>ZoneGroup"`
	}
	if err := xml.Unmarshal([]byte(field(body, "ZoneGroupState")), &state); err != nil {
		return nil, fmt.Errorf("parse zone group state: %w", err)
	}

	// The zone group state has no model name; fetch each member's device
	// description in parallel (local HTTP, a few ms each).
	models := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, zg := range state.Groups {
		for _, m := range zg.Members {
			if m.Invisible == "1" {
				continue
			}
			wg.Add(1)
			go func(uuid, loc string) {
				defer wg.Done()
				if d, err := describe(ctx, loc); err == nil {
					mu.Lock()
					models[uuid] = d.Model
					mu.Unlock()
				}
			}(m.UUID, m.Location)
		}
	}
	wg.Wait()

	var groups []Group
	for _, zg := range state.Groups {
		var g Group
		var names []string
		for _, m := range zg.Members {
			if m.Invisible == "1" {
				continue // bonded satellite
			}
			u, err := url.Parse(m.Location)
			if err != nil {
				continue
			}
			sp := Speaker{IP: u.Hostname(), Room: m.ZoneName, UUID: m.UUID, Model: models[m.UUID]}
			g.Members = append(g.Members, sp)
			names = append(names, m.ZoneName)
			if m.UUID == zg.Coordinator {
				g.Coordinator = sp
			}
		}
		if g.Coordinator.UUID == "" {
			continue
		}
		sort.Strings(names)
		g.Name = strings.Join(names, " + ")
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups, nil
}

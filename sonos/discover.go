package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Speaker is a Sonos device found on the local network.
type Speaker struct {
	IP    string
	Room  string
	UUID  string // e.g. RINCON_000E58XXXXXX01400
	Model string
}

// baseURL is where the speaker's UPnP services live. IP may carry a port
// (host:port), which tests use to talk to a local fake.
func (s Speaker) baseURL() string {
	host := s.IP
	if !strings.Contains(host, ":") {
		host += ":1400"
	}
	return "http://" + host
}

const ssdpAddr = "239.255.255.250:1900"

// Discover sends an SSDP M-SEARCH and returns all speakers that answer within timeout.
func Discover(ctx context.Context, timeout time.Duration) ([]Speaker, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}
	msg := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: " + ssdpAddr + "\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 1\r\n" +
		"ST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n\r\n"
	// Send twice; UDP is lossy.
	for i := 0; i < 2; i++ {
		if _, err := conn.WriteTo([]byte(msg), dst); err != nil {
			return nil, err
		}
	}

	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)
	locations := map[string]bool{}
	buf := make([]byte, 2048)
	for ctx.Err() == nil {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break // deadline reached
		}
		for _, line := range strings.Split(string(buf[:n]), "\r\n") {
			if len(line) > 9 && strings.EqualFold(line[:9], "location:") {
				locations[strings.TrimSpace(line[9:])] = true
			}
		}
	}

	seen := map[string]bool{}
	var out []Speaker
	for loc := range locations {
		sp, err := describe(ctx, loc)
		if err != nil || seen[sp.UUID] {
			continue
		}
		seen[sp.UUID] = true
		out = append(out, sp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Room < out[j].Room })
	return out, nil
}

func describe(ctx context.Context, location string) (Speaker, error) {
	u, err := url.Parse(location)
	if err != nil {
		return Speaker{}, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return Speaker{}, err
	}
	defer resp.Body.Close()

	var d struct {
		Device struct {
			RoomName  string `xml:"roomName"`
			UDN       string `xml:"UDN"`
			ModelName string `xml:"modelName"`
		} `xml:"device"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&d); err != nil {
		return Speaker{}, fmt.Errorf("decode %s: %w", location, err)
	}
	return Speaker{
		IP:    u.Hostname(),
		Room:  d.Device.RoomName,
		UUID:  strings.TrimPrefix(d.Device.UDN, "uuid:"),
		Model: d.Device.ModelName,
	}, nil
}

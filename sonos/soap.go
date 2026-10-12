package sonos

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"time"
)

const (
	avTransportPath = "/MediaRenderer/AVTransport/Control"
	avTransportNS   = "urn:schemas-upnp-org:service:AVTransport:1"
	contentDirPath  = "/MediaServer/ContentDirectory/Control"
	contentDirNS    = "urn:schemas-upnp-org:service:ContentDirectory:1"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// arg is an ordered SOAP argument (order matters to Sonos).
type arg struct{ Name, Value string }

// call performs a SOAP action and returns the raw response body.
func (s Speaker) call(ctx context.Context, path, ns, action string, args ...arg) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:%s xmlns:u="%s">`, action, ns)
	for _, a := range args {
		fmt.Fprintf(&b, "<%s>%s</%s>", a.Name, html.EscapeString(a.Value), a.Name)
	}
	fmt.Fprintf(&b, `</u:%s></s:Body></s:Envelope>`, action)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL()+path, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", fmt.Sprintf(`"%s#%s"`, ns, action))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d: %s", action, resp.StatusCode, body)
	}
	return body, nil
}

// field extracts the text of the first element with the given local name.
func field(body []byte, name string) string {
	d := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := d.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == name {
			var v string
			_ = d.DecodeElement(&v, &se)
			return v
		}
	}
}

package sonos

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSpeaker is a Sonos speaker's AVTransport service. It records every
// action it receives, and refuses the first failRelSeeks time-seeks like a
// speaker that has not started playing yet.
type fakeSpeaker struct {
	mu           sync.Mutex
	actions      []string
	failRelSeeks int
}

var (
	unitRe   = regexp.MustCompile(`<Unit>([^<]*)</Unit>`)
	targetRe = regexp.MustCompile(`<Target>([^<]*)</Target>`)
)

const didl = `&lt;DIDL-Lite xmlns:dc=&quot;http://purl.org/dc/elements/1.1/&quot; xmlns:upnp=&quot;urn:schemas-upnp-org:metadata-1-0/upnp/&quot; xmlns=&quot;urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/&quot;&gt;` +
	`&lt;item id=&quot;1&quot;&gt;&lt;dc:title&gt;Kapitel 2&lt;/dc:title&gt;&lt;dc:creator&gt;Conni&lt;/dc:creator&gt;&lt;upnp:album&gt;Folge 1&lt;/upnp:album&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;`

func (f *fakeSpeaker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	action := r.Header.Get("SOAPAction")
	action = strings.Trim(action[strings.Index(action, "#")+1:], `"`)
	entry := action
	if action == "Seek" {
		entry = fmt.Sprintf("Seek(%s=%s)", unitRe.FindStringSubmatch(string(body))[1], targetRe.FindStringSubmatch(string(body))[1])
	}
	f.mu.Lock()
	f.actions = append(f.actions, entry)
	failing := strings.HasPrefix(entry, "Seek(REL_TIME") && f.failRelSeeks > 0
	if failing {
		f.failRelSeeks--
	}
	f.mu.Unlock()

	if failing {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><detail><UPnPError><errorCode>701</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
		return
	}
	var inner string
	switch action {
	case "GetTransportInfo":
		inner = `<CurrentTransportState>PLAYING</CurrentTransportState>`
	case "GetPositionInfo":
		inner = `<Track>3</Track><TrackDuration>0:10:40</TrackDuration><TrackMetaData>` + didl + `</TrackMetaData><RelTime>0:02:05</RelTime>`
	case "GetMediaInfo":
		inner = `<NrTracks>10</NrTracks>`
	}
	fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:%sResponse>%s</u:%sResponse></s:Body></s:Envelope>`, action, inner, action)
}

func (f *fakeSpeaker) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.actions...)
}

func newFake(t *testing.T, failRelSeeks int) (Speaker, *fakeSpeaker) {
	t.Helper()
	f := &fakeSpeaker{failRelSeeks: failRelSeeks}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return Speaker{IP: strings.TrimPrefix(srv.URL, "http://"), UUID: "RINCON_TEST"}, f
}

func TestStatusReportsTrackAndPosition(t *testing.T) {
	sp, _ := newFake(t, 0)
	st, err := sp.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "PLAYING" || st.Title != "Kapitel 2" || st.Album != "Folge 1" || st.Artist != "Conni" {
		t.Errorf("track details: %+v", st)
	}
	if st.Track != 3 || st.NumTracks != 10 || st.PositionSec != 125 || st.DurationSec != 640 {
		t.Errorf("progress: track %d of %d, %ds of %ds", st.Track, st.NumTracks, st.PositionSec, st.DurationSec)
	}
}

var acc = Account{Service: Spotify, SN: "228", Token: "tok"}
var album = Ref{Service: Spotify, Kind: Album, ID: "abc"}

func TestPlayFromSeeksToTheTrackThenTheTime(t *testing.T) {
	sp, f := newFake(t, 2) // the speaker refuses the time seek twice while it starts up
	err := sp.PlayFrom(context.Background(), acc, album, &Resume{Track: 3, Position: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(f.log(), " > ")
	// The track is chosen before playing; the time can only be set once it plays, and is retried until it works.
	want := "RemoveAllTracksFromQueue > AddURIToQueue > SetAVTransportURI > Seek(TRACK_NR=3) > Play > Seek(REL_TIME=0:02:00) > Seek(REL_TIME=0:02:00) > Seek(REL_TIME=0:02:00)"
	if got != want {
		t.Errorf("commands\n got %s\nwant %s", got, want)
	}
}

func TestPlayFromStillPlaysWhenTheSpeakerWontSeek(t *testing.T) {
	sp, f := newFake(t, 1000) // every time seek is refused
	if err := sp.PlayFrom(context.Background(), acc, album, &Resume{Track: 1, Position: 90 * time.Second}); err != nil {
		t.Fatalf("a refused seek must not fail playback: %v", err)
	}
	log := f.log()
	if !strings.Contains(strings.Join(log, ">"), "Play") {
		t.Errorf("playback never started: %v", log)
	}
	if strings.Contains(strings.Join(log, ">"), "TRACK_NR") {
		t.Errorf("track 1 needs no track seek: %v", log)
	}
}

func TestPlayWithoutResumeNeverSeeks(t *testing.T) {
	for name, resume := range map[string]*Resume{
		"no resume":         nil,
		"start of track 1":  {Track: 1, Position: 0},
		"under a second in": {Track: 1, Position: 400 * time.Millisecond},
	} {
		sp, f := newFake(t, 0)
		if err := sp.PlayFrom(context.Background(), acc, album, resume); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := strings.Join(f.log(), " > "); strings.Contains(got, "Seek") {
			t.Errorf("%s: unexpected seek: %s", name, got)
		}
	}
}

func TestClock(t *testing.T) {
	for in, want := range map[string]int{"0:02:05": 125, "1:00:00": 3600, "0:00:00": 0, "NOT_IMPLEMENTED": 0, "": 0, "1:2": 0, "a:b:c": 0} {
		if got := parseClock(in); got != want {
			t.Errorf("parseClock(%q) = %d, want %d", in, got, want)
		}
	}
	for d, want := range map[time.Duration]string{125 * time.Second: "0:02:05", 3600 * time.Second: "1:00:00", 3725*time.Second + 600*time.Millisecond: "1:02:06"} {
		if got := formatClock(d); got != want {
			t.Errorf("formatClock(%v) = %q, want %q", d, got, want)
		}
	}
}

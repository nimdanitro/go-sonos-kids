package server

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/nimdanitro/go-sonos-kids/meta"
	"github.com/nimdanitro/go-sonos-kids/sonos"
	"github.com/nimdanitro/go-sonos-kids/store"
)

func reading(track, pos, dur int) PlayerStatus {
	return PlayerStatus{
		State:       "PLAYING",
		Album:       "Folge 1",
		Title:       fmt.Sprintf("Kapitel %d", track),
		Track:       track,
		NumTracks:   10,
		PositionSec: pos,
		DurationSec: dur,
	}
}

func TestUpdateProgressFollowsTheListener(t *testing.T) {
	var p store.Progress

	p = updateProgress(p, reading(1, 47, 120))
	if p.Last != 1 || p.Total != 10 || p.Tracks[1].Position != 45 || p.Tracks[1].Title != "Kapitel 1" ||
		p.Tracks[1].Duration != 120 ||
		p.Tracks[1].Done {
		t.Errorf("first reading (positions are stored in 5 s steps): %+v", p)
	}
	// A reading inside the same step changes nothing, so polling does not cause writes.
	if again := updateProgress(p, reading(1, 49, 120)); fmt.Sprint(again) != fmt.Sprint(p) {
		t.Errorf("49 s should look the same as 47 s:\n%+v\n%+v", again, p)
	}

	// Reaching the end of a track marks it done (with the exact position), and moving on keeps that.
	p = updateProgress(p, reading(1, 112, 120))
	if !p.Tracks[1].Done || p.Tracks[1].Position != 112 {
		t.Errorf("a track near its end is done: %+v", p.Tracks[1])
	}

	p = updateProgress(p, reading(2, 3, 600))
	if p.Last != 2 || !p.Tracks[1].Done || p.Tracks[2].Done {
		t.Errorf("moving to track 2: %+v", p)
	}

	// Skipping ahead with "next" does not count the skipped track as heard.
	p = updateProgress(p, reading(2, 100, 600))

	p = updateProgress(p, reading(3, 0, 640))
	if p.Tracks[2].Done {
		t.Errorf("track 2 was skipped, not heard: %+v", p.Tracks[2])
	}

	// A reading without a track number (e.g. radio) is ignored.
	if same := updateProgress(p, PlayerStatus{State: "PLAYING"}); same.Last != 3 {
		t.Errorf("no track number: %+v", same)
	}
}

func TestUpdateProgressCompletesOnlyWhenTheLastTrackIsHeard(t *testing.T) {
	p := updateProgress(store.Progress{}, reading(9, 590, 600))
	if p.Completed {
		t.Error("track 9 of 10 is not the end")
	}

	p = updateProgress(p, reading(10, 300, 600))
	if p.Completed {
		t.Error("halfway through the last track is not the end")
	}

	p = updateProgress(p, reading(10, 590, 600))
	if !p.Completed {
		t.Errorf("the end of the last track completes the item: %+v", p)
	}
	// It stays completed even if the speaker then wanders back to the start.
	if p = updateProgress(p, reading(1, 5, 120)); !p.Completed {
		t.Error("completed is sticky")
	}
}

func TestUpdateProgressDoesNotShareItsInput(t *testing.T) {
	before := updateProgress(store.Progress{}, reading(1, 30, 600))
	snapshot := fmt.Sprint(before)

	_ = updateProgress(before, reading(2, 10, 600))
	if fmt.Sprint(before) != snapshot {
		t.Error("updateProgress changed the progress it was given")
	}
}

func TestShortTracksGetASmallerDoneMargin(t *testing.T) {
	if nearEnd(10, 40) { // 40 s long: margin is 4 s, not 15
		t.Error("10 s into a 40 s track is not the end")
	}

	if !nearEnd(37, 40) {
		t.Error("37 s into a 40 s track is the end")
	}

	if nearEnd(5, 0) {
		t.Error("an unknown duration never counts as the end")
	}
}

func TestResumePoint(t *testing.T) {
	prog := func(last, total, pos int, done, completed bool) store.Progress {
		return store.Progress{
			Last: last, Total: total, Completed: completed,
			Tracks: map[int]store.TrackProgress{last: {Position: pos, Duration: 600, Done: done}},
		}
	}

	cases := []struct {
		name string
		p    store.Progress
		want *sonos.Resume
	}{
		{"nothing heard yet", store.Progress{}, nil},
		{"finished", prog(10, 10, 595, true, true), nil},
		{"barely started", prog(1, 10, 12, false, false), nil}, // 12 - 5 s rewind is under the minimum
		{"a while into track 1", prog(1, 10, 120, false, false), &sonos.Resume{Track: 1, Position: 115 * time.Second}},
		{"mid track 3", prog(3, 10, 125, false, false), &sonos.Resume{Track: 3, Position: 120 * time.Second}},
		{"just started track 4", prog(4, 10, 3, false, false), &sonos.Resume{Track: 4, Position: 0}},
		{"stopped between tracks", prog(3, 10, 598, true, false), &sonos.Resume{Track: 4}},
		{"stopped after the last track", prog(10, 10, 598, true, false), nil},
	}
	for _, c := range cases {
		got := resumePoint(c.p)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestProgressFractionForTheUI(t *testing.T) {
	p := store.Progress{Last: 3, Total: 10, Tracks: map[int]store.TrackProgress{3: {Position: 300, Duration: 600}}}
	if f := toProgress(p).Fraction; math.Abs(f-0.25) > 1e-9 { // two tracks before it, half of the third, over ten
		t.Errorf("fraction %v, want 0.25", f)
	}

	if f := toProgress(store.Progress{Last: 2, Completed: true, Total: 10}).Fraction; f != 1 {
		t.Errorf("completed fraction %v", f)
	}

	if f := toProgress(
		store.Progress{Last: 1, Tracks: map[int]store.TrackProgress{1: {Position: 60, Duration: 120}}},
	).Fraction; f != 0.5 {
		t.Errorf("unknown total falls back to the current track: %v", f)
	}
}

// The whole flow through the HTTP API: listen, stop, come back.
func TestAudiobookResumesWhereTheKidStopped(t *testing.T) {
	e := newEnv(t, "secret", meta.Info{Title: "Folge 1", Artist: "Conni", ReleaseDate: "2008-03-01"})
	h, _ := e.expect(201, "POST", "/api/admin/profiles", "secret", map[string]any{"name": "Globi", "speaker": "Globi"})
	pid := idOf(h)
	add := func(ref, category string) int {
		res, _ := e.expect(
			201,
			"POST",
			"/api/admin/items",
			"secret",
			map[string]any{"url": ref, "category": category, "profile_ids": []int{pid}},
		)

		return idOf(res["items"].([]any)[0].(map[string]any))
	}
	book := add("spotify:album:aaa", "audiobook")
	music := add("spotify:album:bbb", "music")

	progressOf := func(category string, id int) map[string]any {
		t.Helper()

		_, raw := e.expect(200, "GET", "/api/profiles/"+itoa(pid)+"/library?category="+category, "", nil)

		var items []map[string]any

		_ = json.Unmarshal(raw, &items)
		for _, it := range items {
			if idOf(it) == id {
				pr, _ := it["progress"].(map[string]any)
				return pr
			}
		}

		t.Fatalf("item %d not in the library", id)

		return nil
	}
	poll := func(st PlayerStatus) { // the player screen asking for the status
		t.Helper()

		e.player.status = st
		e.expect(200, "GET", "/api/profiles/"+itoa(pid)+"/status", "", nil)
	}
	play := func(id int, restart bool) map[string]any {
		t.Helper()

		res, _ := e.expect(
			200,
			"POST",
			"/api/profiles/"+itoa(pid)+"/play",
			"",
			map[string]any{"item_id": id, "restart": restart},
		)

		return res
	}
	lastResume := func() *sonos.Resume { return e.player.resumes[len(e.player.resumes)-1] }
	// itemID is what the status offers for "start over": the audiobook playing now, or nothing.
	itemID := func() any {
		m, _ := e.expect(200, "GET", "/api/profiles/"+itoa(pid)+"/status", "", nil)
		return m["item_id"]
	}

	// First listen: starts from the beginning, nothing to resume.
	if res := play(book, false); res["resumed"] != false || lastResume() != nil {
		t.Fatalf("first play: %v resume=%+v", res, lastResume())
	}

	if progressOf("audiobook", book) != nil {
		t.Error("no progress before anything was heard")
	}

	poll(reading(1, 37, 600))
	poll(reading(3, 125, 640))

	pr := progressOf("audiobook", book)
	if pr == nil || pr["track"] != 3.0 || pr["total"] != 10.0 || pr["position"] != 125.0 ||
		pr["title"] != "Kapitel 3" ||
		pr["completed"] != false {
		t.Fatalf("progress after listening: %v", pr)
	}

	if f := pr["fraction"].(float64); math.Abs(f-(2+125.0/640)/10) > 1e-9 {
		t.Errorf("fraction %v", f)
	}

	if got := itemID(); got != float64(book) {
		t.Errorf("while the audiobook plays the status names it: %v, want %d", got, book)
	}

	// Pausing is recorded; a stopped speaker (which reports no useful position) is not.
	paused := reading(3, 131, 640)
	paused.State = "PAUSED_PLAYBACK"
	poll(paused)

	stopped := reading(1, 0, 600)
	stopped.State = "STOPPED"
	poll(stopped)

	if pr = progressOf("audiobook", book); pr["track"] != 3.0 || pr["position"] != 130.0 {
		t.Errorf("after pause and stop: %v", pr)
	}

	// Coming back continues at track 3, five seconds before where they stopped.
	if res := play(book, false); res["resumed"] != true {
		t.Errorf("second play should resume: %v", res)
	}

	if r := lastResume(); r == nil || r.Track != 3 || r.Position != 125*time.Second {
		t.Errorf("resume point: %+v", r)
	}

	// If the speaker is then used for something else (the Sonos app), that is not this book's progress.
	poll(reading(3, 140, 640))
	other := reading(1, 20, 300)
	other.Album = "Some Other Album"
	poll(other)
	poll(reading(1, 30, 300)) // still the other album: no longer tracked

	if got := itemID(); got != nil {
		t.Errorf("another album is playing, so there is nothing to restart: %v", got)
	}

	if pr = progressOf("audiobook", book); pr["track"] != 3.0 || pr["position"] != 140.0 {
		t.Errorf("another album must not overwrite the progress: %v", pr)
	}

	// Finishing the last track completes it, and the next play starts afresh.
	play(book, false)
	poll(reading(10, 595, 600))

	if pr = progressOf("audiobook", book); pr["completed"] != true || pr["fraction"] != 1.0 {
		t.Errorf("finished book: %v", pr)
	}

	if res := play(book, false); res["resumed"] != false || lastResume() != nil {
		t.Errorf("a finished book starts over: %v %+v", res, lastResume())
	}

	if progressOf("audiobook", book) != nil {
		t.Error("starting over forgets the finished run")
	}

	// "restart" skips resuming even when there is progress.
	poll(reading(1, 10, 600))
	poll(reading(4, 60, 600))

	if res := play(book, true); res["resumed"] != false || lastResume() != nil {
		t.Errorf("restart: %v %+v", res, lastResume())
	}

	if progressOf("audiobook", book) != nil {
		t.Error("restart forgets the earlier progress")
	}

	// Music is never tracked or resumed.
	play(book, false)

	if got := itemID(); got != float64(book) {
		t.Fatalf("precondition: the audiobook is offered for restart: %v", got)
	}

	play(music, false)
	poll(reading(2, 50, 200))

	if got := itemID(); got != nil {
		t.Errorf("music has nothing to restart, but the status offers %v", got)
	}

	if progressOf("music", music) != nil {
		t.Error("music must not get progress")
	}

	if lastResume() != nil {
		t.Error("music must not resume")
	}

	// Marking an audiobook unplayed in the admin forgets its progress too.
	play(book, false)
	poll(reading(2, 200, 600))

	if progressOf("audiobook", book) == nil {
		t.Fatal("expected progress to exist")
	}

	e.expect(200, "PUT", "/api/admin/items/"+itoa(book), "secret", map[string]any{"played": false})

	if progressOf("audiobook", book) != nil {
		t.Error("marking unplayed must forget the progress")
	}
}

func TestFractionOfTheQueue(t *testing.T) {
	cases := []struct {
		name   string
		st     PlayerStatus
		want   float64
		wantOK bool
	}{
		{
			"halfway through track 3 of 10",
			PlayerStatus{Track: 3, NumTracks: 10, PositionSec: 320, DurationSec: 640},
			0.25,
			true,
		},
		{"start of the first track", PlayerStatus{Track: 1, NumTracks: 4, PositionSec: 0, DurationSec: 200}, 0, true},
		{"end of the last track", PlayerStatus{Track: 4, NumTracks: 4, PositionSec: 200, DurationSec: 200}, 1, true},
		{"queue size unknown: just this track", PlayerStatus{Track: 2, PositionSec: 50, DurationSec: 200}, 0.25, true},
		{"never above 1", PlayerStatus{Track: 4, NumTracks: 4, PositionSec: 999, DurationSec: 200}, 1, true},
		{"radio has no track", PlayerStatus{Track: 0, PositionSec: 5, DurationSec: 0}, 0, false},
		{"unknown track length", PlayerStatus{Track: 2, NumTracks: 4, PositionSec: 5}, 0, false},
	}
	for _, c := range cases {
		got, ok := fractionOf(c.st)
		if ok != c.wantOK || math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: got %v,%v want %v,%v", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestStatusEndpointReportsTheFraction(t *testing.T) {
	e := newEnv(t, "secret", meta.Info{})
	h, _ := e.expect(201, "POST", "/api/admin/profiles", "secret", map[string]any{"name": "Globi", "speaker": "Globi"})
	path := "/api/profiles/" + itoa(idOf(h)) + "/status"

	e.player.status = reading(3, 320, 640)

	got, _ := e.expect(200, "GET", path, "", nil)
	if got["fraction"] != 0.25 || got["track"] != 3.0 || got["num_tracks"] != 10.0 || got["position"] != 320.0 ||
		got["duration"] != 640.0 {
		t.Errorf("status: %v", got)
	}

	e.player.status = PlayerStatus{State: "PLAYING", Title: "Radio", Album: "x"} // no track information

	got, _ = e.expect(200, "GET", path, "", nil)
	if _, has := got["fraction"]; has {
		t.Errorf("a stream has no fraction: %v", got)
	}
}

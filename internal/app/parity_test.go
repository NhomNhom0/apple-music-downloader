package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amdl/internal/config"
	"amdl/internal/model"
)

func captureDispatch(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = old; reader.Close() }()
	fn()
	writer.Close()
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(result)
}

func TestCollectionDispatchStopAndContinue(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		stop                              bool
		wantTotal, wantSuccess, wantError int
	}{
		{"album-stop", true, 2, 1, 1}, {"playlist-continue", false, 4, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// Already saved first song exercises the production success path without media.
			if err := os.WriteFile(filepath.Join(dir, "first.m4a"), []byte("offline fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			r := NewRunner(config.ConfigSet{StopOnTrackError: tc.stop, AacType: "aac-lc", SongFileFormat: "{SongName}", LimitMax: 200, LrcFormat: "lrc"})
			r.Flags.AAC = true
			tracks := []model.Track{
				{ID: "one", Type: "songs", SaveDir: dir, TaskNum: 1, TaskTotal: 4},
				{ID: "two", Type: "songs", SaveDir: dir, TaskNum: 2, TaskTotal: 4},
				{ID: "video", Type: "music-videos", SaveDir: dir, TaskNum: 3, TaskTotal: 4},
				{ID: "four", Type: "songs", SaveDir: dir, TaskNum: 4, TaskTotal: 4},
			}
			for i, name := range []string{"first", "second", "video", "fourth"} {
				tracks[i].Resp.Attributes.Name = name
			}
			// This is the same helper invoked by album, playlist and station dispatch.
			out := captureDispatch(t, func() { r.processSelectedTracks(tracks, []int{1, 2, 3, 4}, nil, "", "", nil) })
			if c := r.State.Counter; c.Total != tc.wantTotal || c.Success != tc.wantSuccess || c.Error != tc.wantError {
				t.Fatalf("counters=%+v, output=%q", c, out)
			}
			markers := []string{}
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "AMDL_TRACK_ERROR ") {
					markers = append(markers, strings.TrimPrefix(line, "AMDL_TRACK_ERROR "))
				}
			}
			if tc.stop {
				if len(markers) != 1 || strings.Contains(out, "Track 3 of 4") {
					t.Fatalf("stop marker/dispatch: %q", out)
				}
				var payload map[string]interface{}
				if err := json.Unmarshal([]byte(markers[0]), &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload) != 4 || payload["id"] != "two" || payload["name"] != "second" || payload["track"] != float64(2) || payload["total"] != float64(4) {
					t.Fatalf("marker=%v", payload)
				}
			} else if len(markers) != 0 || !strings.Contains(out, "Track 4 of 4") {
				t.Fatalf("continue marker/dispatch: %q", out)
			}
		})
	}
}

func TestCollectionDispatchAudioOnlyVideoIsNotFailure(t *testing.T) {
	r := NewRunner(config.ConfigSet{AudioOnly: true, StopOnTrackError: true, AacType: "aac-lc"})
	r.Flags.AAC = true
	tracks := []model.Track{{ID: "mv", Type: "music-videos", TaskNum: 1, TaskTotal: 2}, {ID: "song", Type: "songs", TaskNum: 2, TaskTotal: 2}}
	tracks[1].Resp.Attributes.Name = "later song"
	out := captureDispatch(t, func() { r.processSelectedTracks(tracks, []int{1, 2}, nil, "", "", nil) })
	if r.State.Counter.NotSong != 1 || r.State.Counter.Error != 1 || r.State.Counter.Total != 2 || strings.Count(out, "AMDL_TRACK_ERROR ") != 1 || !strings.Contains(out, "Track 2 of 2") {
		t.Fatalf("audio-only did not skip/continue: %+v %q", r.State.Counter, out)
	}
}

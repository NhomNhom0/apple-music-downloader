package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"amdl/internal/config"
	"amdl/internal/model"
)

func TestTrackErrorMarkerWorkerProtocol(t *testing.T) {
	track := &model.Track{ID: "song-3", TaskNum: 3, TaskTotal: 4}
	track.Resp.Attributes.Name = "Title with \"quotes\""
	line := trackErrorMarker(track)
	if !strings.HasPrefix(line, "AMDL_TRACK_ERROR ") || strings.Contains(line, "\n") {
		t.Fatalf("invalid marker envelope: %q", line)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "AMDL_TRACK_ERROR ")), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 4 || payload["id"] != "song-3" || payload["track"] != float64(3) || payload["total"] != float64(4) || payload["name"] != track.Resp.Attributes.Name {
		t.Fatalf("worker marker fields: %#v", payload)
	}
}

func TestConversionFailureCountsAsTrackError(t *testing.T) {
	r := NewRunner(config.ConfigSet{ConvertAfterDownload: true, ConvertFormat: "flac", FFmpegPath: "definitely-not-a-ffmpeg-binary"})
	track := &model.Track{SavePath: filepath.Join(t.TempDir(), "source.m4a")}
	if err := r.convertIfNeeded(track); err == nil {
		t.Fatal("conversion without ffmpeg unexpectedly succeeded")
	}
}

func TestStopOnTrackErrorFailureMarkerAndCounter(t *testing.T) {
	r := NewRunner(config.ConfigSet{StopOnTrackError: true, AacType: "aac-lc"})
	r.Flags.AAC = true
	track := &model.Track{ID: "song-3", Type: "songs", TaskNum: 3, TaskTotal: 4}
	track.Resp.Attributes.Name = "Unavailable song"
	// Missing lite-server fails before media/network calls, exercising the
	// same track error counter consumed by album and playlist loop stop gates.
	r.ripTrack(track, "", "")
	if r.State.Counter.Error != 1 || r.State.Counter.Total != 1 || r.State.Counter.Success != 0 {
		t.Fatalf("stop failure counters: %+v", r.State.Counter)
	}
}

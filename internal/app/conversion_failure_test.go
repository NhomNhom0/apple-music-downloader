package app

import (
	"os"
	"path/filepath"
	"testing"

	"amdl/internal/config"
	"amdl/internal/model"
)

func TestConversionFailurePreservesOriginal(t *testing.T) {
	for _, tc := range []struct {
		name, script, format string
		check                bool
	}{
		{"process exit", "exit 1\n", "flac", false},
		{"bad ALAC stderr", "echo synthetic-decode-error >&2\nexit 0\n", "flac", true},
		{"unsupported format", "exit 0\n", "invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "source.m4a")
			tool := filepath.Join(dir, "ffmpeg")
			if err := os.WriteFile(src, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(tool, []byte("#!/bin/sh\n"+tc.script), 0700); err != nil {
				t.Fatal(err)
			}
			r := NewRunner(config.ConfigSet{ConvertAfterDownload: true, ConvertFormat: tc.format, FFmpegPath: tool, ConvertCheckBadALAC: tc.check})
			track := &model.Track{SavePath: src, SaveName: "source.m4a"}
			if err := r.convertIfNeeded(track); err == nil {
				t.Fatal("conversion error swallowed")
			}
			got, err := os.ReadFile(src)
			if err != nil || string(got) != "original" || track.SavePath != src {
				t.Fatalf("original/path changed on failure: %v %+v", err, track)
			}
		})
	}
}

func TestUnavailableTrackStopsOnlyWhenConfigured(t *testing.T) {
	for _, stop := range []bool{true, false} {
		r := NewRunner(config.ConfigSet{StopOnTrackError: stop})
		r.Flags.Atmos = true
		tracks := []model.Track{{ID: "one", Type: "songs", TaskNum: 1, TaskTotal: 2}, {ID: "two", Type: "songs", TaskNum: 2, TaskTotal: 2}}
		out := captureDispatch(t, func() { r.processSelectedTracks(tracks, []int{1, 2}, nil, "", "") })
		if stop {
			if r.State.Counter.Total != 1 || r.State.Counter.Error != 1 || r.State.Counter.Unavailable != 1 {
				t.Fatalf("stop counters=%+v %q", r.State.Counter, out)
			}
		} else if r.State.Counter.Total != 2 || r.State.Counter.Error != 0 || r.State.Counter.Unavailable != 2 {
			t.Fatalf("continue counters=%+v %q", r.State.Counter, out)
		}
	}
}

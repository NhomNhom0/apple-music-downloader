package app

import (
	"amdl/internal/config"
	"amdl/internal/model"
	"testing"
)

func TestAudioOnlySkipsVideoWithoutNetwork(t *testing.T) {
	r := NewRunner(config.ConfigSet{AudioOnly: true, LiteServer: "http://127.0.0.1:1"})
	r.ripTrack(&model.Track{Type: "music-videos", ID: "1", Name: "Video"}, "", "")
	if r.State.Counter.Total != 0 || r.State.Counter.Error != 0 || r.State.Counter.Success != 0 {
		t.Fatal("skipped video changed audio counters")
	}
}
func TestAudioOnlyStillProcessesAudio(t *testing.T) {
	r := NewRunner(config.ConfigSet{AudioOnly: true})
	r.ripTrack(&model.Track{Type: "songs", ID: "1", SaveDir: t.TempDir()}, "", "")
	if r.State.Counter.Total != 1 || r.State.Counter.Error != 1 {
		t.Fatal("audio was silently skipped")
	}
}

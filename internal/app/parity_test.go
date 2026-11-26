package app

import (
	"testing"

	"amdl/internal/config"
	"amdl/internal/model"
)

func TestAudioOnlySkipsMusicVideoWithoutError(t *testing.T) {
	r := NewRunner(config.ConfigSet{AudioOnly: true, LiteServer: "https://must-not-be-contacted.invalid"})
	r.ripTrack(&model.Track{Type: "music-videos", ID: "video-1"}, "", "")
	if r.State.Counter.Total != 1 || r.State.Counter.NotSong != 1 || r.State.Counter.Error != 0 || r.State.Counter.Success != 0 {
		t.Fatalf("unexpected skipped video counters: %+v", r.State.Counter)
	}
}

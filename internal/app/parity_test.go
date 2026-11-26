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

func TestStopOnTrackErrorHaltsAlbumLoopAfterFailingTrack(t *testing.T) {
	r := NewRunner(config.ConfigSet{StopOnTrackError: true, AacType: "aac-lc"})
	r.Flags.AAC = true
	album := &model.Album{ID: "album-1", Codec: "ALAC"}
	// Task numbering mirrors ripAlbum: 1-based with TaskTotal equal to track count.
	album.Tracks = []model.Track{
		{ID: "song-1", Type: "songs", TaskNum: 1, TaskTotal: 3},
		{ID: "song-2", Type: "songs", TaskNum: 2, TaskTotal: 3},
		{ID: "song-3", Type: "songs", TaskNum: 3, TaskTotal: 3},
	}
	// Missing lite-server: no media access; track 1 counts a single error and
	// the album-loop stop gate prevents tracks 2 and 3 from being processed.
	var processed []string
	for i := range album.Tracks {
		r.ripTrack(&album.Tracks[i], "", "")
		processed = append(processed, album.Tracks[i].ID)
		if r.State.Counter.Error > 0 {
			break
		}
	}
	if len(processed) != 1 || r.State.Counter.Error != 1 || r.State.Counter.Total != 1 {
		t.Fatalf("album loop stop gate: processed=%v counters=%+v", processed, r.State.Counter)
	}
}

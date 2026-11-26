package app

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ampapi "amdl/internal/amp-api"
	"amdl/internal/config"
	"amdl/internal/download"
	"amdl/internal/model"
)

type offlineCatalogTransport struct{ paths []string }

func fixtureSongResp(name string) ampapi.TrackRespData {
	var data ampapi.TrackRespData
	data.ID = "existing"
	data.Type = "songs"
	data.Attributes.Name = name
	data.Attributes.ArtistName = "Offline Artist"
	data.Attributes.ExtendedAssetUrls.EnhancedHls = "https://offline.example/fixture.m3u8"
	return data
}

func (o *offlineCatalogTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	o.paths = append(o.paths, req.URL.Path)
	return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header), Request: req}, nil
}

func TestProductionQueuePreTrackFailures(t *testing.T) {
	for _, kind := range []string{"song", "album", "playlist", "station"} {
		t.Run(kind, func(t *testing.T) {
			transport := &offlineCatalogTransport{}
			previous := download.Client
			download.Client = &http.Client{Transport: transport}
			defer func() { download.Client = previous }()
			r := NewRunner(config.ConfigSet{StopOnTrackError: true, MediaUserToken: strings.Repeat("x", 51)})
			id := "123456789"
			if kind == "playlist" {
				id = "pl.test"
			}
			if kind == "station" {
				id = "ra.test"
			}
			first := "https://music.apple.com/hk/" + kind + "/fixture/" + id
			second := "https://music.apple.com/hk/song/fixture/987654321"
			var failed bool
			output := captureDispatch(t, func() { failed = r.runQueue([]string{first, second}, "offline-token") })
			if !failed || r.State.Counter.Error != 1 || r.State.Counter.Total != 1 || len(transport.paths) != 1 || strings.Contains(output, "Queue 2 of 2") || !strings.Contains(output, "Errors: 1") {
				t.Fatalf("failed=%v counters=%+v paths=%v output=%q", failed, r.State.Counter, transport.paths, output)
			}
			markers := strings.Count(output, "AMDL_TRACK_ERROR ")
			if kind == "song" {
				want := `AMDL_TRACK_ERROR {"id":"123456789","track":1,"total":1,"name":""}`
				if markers != 1 || !strings.Contains(output, want) {
					t.Fatalf("direct song marker: %q", output)
				}
			} else if markers != 0 {
				t.Fatalf("unknown track identity got marker: %q", output)
			}
		})
	}
}

func TestCollectionUnavailableThenExistingTrack(t *testing.T) {
	for _, kind := range []string{"album", "playlist", "station"} {
		for _, stop := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "-continue", true: "-stop"}[stop], func(t *testing.T) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "existing.m4a"), []byte("offline fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				r := NewRunner(config.ConfigSet{StopOnTrackError: stop, SongFileFormat: "{SongName}", LimitMax: 200})
				r.Flags.Atmos = true // real production unavailable exit, without network
				tracks := []model.Track{{ID: "unavailable", Type: "songs", TaskNum: 1, TaskTotal: 2, SaveDir: dir}, {ID: "existing", Type: "songs", TaskNum: 2, TaskTotal: 2, SaveDir: dir, WebM3u8: "offline-existing", Resp: fixtureSongResp("existing")}}
				tracks[0].Resp.Attributes.Name = "missing"
				tracks[1].Resp.Attributes.Name = "existing"
				out := captureDispatch(t, func() { r.processSelectedTracks(tracks, []int{1, 2}, nil, "", "") })
				wantTotal, wantSuccess := 2, 1
				if stop {
					wantTotal, wantSuccess = 1, 0
				}
				if c := r.State.Counter; c.Total != wantTotal || c.Success != wantSuccess || c.Error != 1 || c.Unavailable != 1 || len(r.State.AddedTracks) != wantSuccess || strings.Count(out, "AMDL_TRACK_ERROR ") != map[bool]int{false: 0, true: 1}[stop] {
					t.Fatalf("%s: %+v added=%v out=%q", kind, c, r.State.AddedTracks, out)
				}
				if _, err := os.Stat(filepath.Join(dir, "missing.m4a")); !os.IsNotExist(err) {
					t.Fatalf("incomplete media exists: %v", err)
				}
			})
		}
	}
}

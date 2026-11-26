package app

import (
	"os"
	"reflect"
	"testing"

	"amdl/internal/config"

	"gopkg.in/yaml.v2"
)

// Mirrors every flat key in the deployed config, using inert synthetic values.
const liveKeyFixture = `
media-user-token: synthetic-media-token
authorization-token: synthetic-authorization
language: en
lrc-extra: synthetic-extra
lrc-type: lyrics
lrc-format: lrc
embed-lrc: true
save-lrc-file: true
save-artist-cover: true
save-animated-artwork: true
emby-animated-artwork: true
embed-cover: true
cover-size: 1x1
cover-format: png
tag-sort-order: true
tag-itunes-id: true
alac-save-folder: synthetic-alac
atmos-save-folder: synthetic-atmos
aac-save-folder: synthetic-aac
mv-save-folder: synthetic-mv
max-memory-limit: 123
lite-server: http://127.0.0.1:1
exit-on-error: true
get-m3u8-mode: all
aac-type: aac
alac-max: 96000
atmos-max: 2448
limit-max: 77
album-folder-format: synthetic-album
playlist-folder-format: synthetic-playlist
song-file-format: synthetic-song
artist-folder-format: synthetic-artist
explicit-choice: synthetic-explicit
clean-choice: synthetic-clean
apple-master-choice: synthetic-master
use-songinfo-for-playlist: true
dl-albumcover-for-playlist: true
mv-audio-type: aac
mv-max: 1080
storefront: zz
alac-fix: true
convert-after-download: true
convert-format: flac
convert-keep-original: true
convert-skip-if-source-matches: true
ffmpeg-path: /synthetic/ffmpeg
convert-extra-args: synthetic-args
convert-with-metadata: true
convert-warn-lossy-to-lossless: true
convert-skip-lossy-to-lossless: true
convert-check-bad-alac: true
convert-delete-bad-alac: true
proxy: http://127.0.0.1:2
audio-only: true
stop-on-track-error: true
`

func TestFlatLiveConfigEveryKeyMapsToConfig(t *testing.T) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(liveKeyFixture), &raw); err != nil {
		t.Fatal(err)
	}
	var cfg config.ConfigSet
	if err := yaml.Unmarshal([]byte(liveKeyFixture), &cfg); err != nil {
		t.Fatal(err)
	}

	typ := reflect.TypeOf(cfg)
	mapped := make(map[string]bool)
	for i := 0; i < typ.NumField(); i++ {
		if key := typ.Field(i).Tag.Get("yaml"); key != "" {
			mapped[key] = true
		}
	}
	for key := range raw {
		if !mapped[key] {
			t.Errorf("live config key %q has no ConfigSet field", key)
		}
	}
	if !cfg.AudioOnly || !cfg.StopOnTrackError || !cfg.ConvertAfterDownload || cfg.LiteServer == "" || cfg.AlacSaveFolder == "" {
		t.Fatalf("critical live values did not parse: %+v", cfg)
	}
}

func TestLoadConfigPreservesFlatConversionPipeline(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("config.yaml", []byte(liveKeyFixture), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(config.ConfigSet{})
	if err := r.loadConfig(); err != nil {
		t.Fatal(err)
	}
	if !r.Config.ConvertAfterDownload || r.Config.ConvertFormat != "flac" || !r.Config.ConvertKeepOriginal || r.Config.FFmpegPath != "/synthetic/ffmpeg" || r.Config.ConvertExtraArgs != "synthetic-args" || !r.Config.ConvertWithMetadata {
		t.Fatalf("conversion config changed: %+v", r.Config)
	}
}

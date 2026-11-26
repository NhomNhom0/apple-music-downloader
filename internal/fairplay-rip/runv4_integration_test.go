package fairplayrip

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"amdl/internal/config"
	"amdl/internal/download"
	temarimod "github.com/WorldObservationLog/Temari/bindings/go"
	"github.com/grafov/m3u8"
	"github.com/itouakirai/mp4ff/mp4"
)

type offlineTransport func(*http.Request) (*http.Response, error)

func (f offlineTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Clear synthetic samples isolate the real producer/cache/worker/writer pipeline
// from codec and cipher fixtures. Templates are real native handles, not mocks.
func syntheticMP4(t *testing.T, count int) []byte {
	t.Helper()
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(48000, "audio", "und")
	if err := init.Moov.Trak.SetAACDescriptor(2, 48000); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := init.Encode(&b); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		frag, err := mp4.CreateFragment(uint32(i+1), 1)
		if err != nil {
			t.Fatal(err)
		}
		data := bytes.Repeat([]byte{byte(i + 1)}, 32)
		frag.AddFullSample(mp4.FullSample{Sample: mp4.Sample{Dur: 1024, Size: uint32(len(data))}, DecodeTime: uint64(i * 1024), Data: data})
		if err := frag.Encode(&b); err != nil {
			t.Fatal(err)
		}
	}
	return b.Bytes()
}

func TestDownloadAndDecryptFileOfflineRotation(t *testing.T) {
	oldLib, oldClient := lib, download.Client
	t.Cleanup(func() { lib, download.Client = oldLib, oldClient })
	var err error
	if path := os.Getenv("TEMARI_LIB"); path != "" {
		lib, err = temarimod.Load(path)
	} else {
		lib, err = temarimod.LoadDefault()
	}
	if err != nil {
		t.Fatalf("native Temari required (set TEMARI_LIB): %v", err)
	}
	var template templateResponse
	if err := json.Unmarshal([]byte(prefetchTemplateJSON), &template); err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(liteResponse{Data: template})
	if err != nil {
		t.Fatal(err)
	}
	for _, memory := range []int{0, 1} {
		for _, tc := range []struct {
			name string
			keys []string
			fail bool
			want []string
		}{
			{"A-A-B-at-index2-A", []string{"A", "A", "B", "A"}, false, []string{"123:A", "123:B"}},
			{"nil-key-inheritance", []string{"A", "", "B", "A"}, false, []string{"123:A", "123:B"}},
			// fetchTemplate's built-in prefetch branch is reachable only with Adam ID 0;
			// any accidental nonzero ID would hit the transport and fail this assertion.
			{"prefetch-ID0", []string{prefetchKey, "", prefetchKey, ""}, false, nil},
			{"failed-B", []string{"A", "A", "B", "A"}, true, []string{"123:A", "123:B"}},
		} {
			t.Run(fmt.Sprintf("%s/memory-%d", tc.name, memory), func(t *testing.T) {
				input := syntheticMP4(t, len(tc.keys))
				var calls []string
				download.Client = &http.Client{Transport: offlineTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.Host != "offline.invalid" || r.URL.Path != "/key" {
						return nil, fmt.Errorf("unexpected offline request: %s", r.URL)
					}
					uri := r.URL.Query().Get("uri")
					calls = append(calls, r.URL.Query().Get("adamId")+":"+uri)
					body := string(envelope)
					if tc.fail && uri == "B" {
						body = `{"code":1,"msg":"synthetic key failure"}`
					}
					return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
				})}
				segments := make([]*m3u8.MediaSegment, len(tc.keys))
				for i, key := range tc.keys {
					segments[i] = &m3u8.MediaSegment{}
					if key != "" {
						segments[i].Key = &m3u8.Key{URI: key}
					}
				}
				out := filepath.Join(t.TempDir(), "track.m4a")
				err := downloadAndDecryptFile("http://offline.invalid", bytes.NewReader(input), out, "123", segments, int64(len(input)), config.ConfigSet{MaxMemoryLimit: memory})
				if !reflect.DeepEqual(calls, tc.want) {
					t.Fatalf("key requests=%v want=%v", calls, tc.want)
				}
				if tc.fail {
					if err == nil || !strings.Contains(err.Error(), "synthetic key failure") {
						t.Fatalf("fetch error=%v", err)
					}
					for _, path := range []string{out, out + ".part"} {
						if _, e := os.Stat(path); !os.IsNotExist(e) {
							t.Fatalf("failed fetch published %s: %v", path, e)
						}
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(out)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, input) {
					t.Fatal("pipeline changed or lost synthetic fragments/order")
				}
			})
		}
	}
}

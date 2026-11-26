package fairplayrip

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"amdl/internal/config"
	"amdl/internal/download"
	"amdl/internal/wrapper"

	"github.com/grafov/m3u8"
	"github.com/itouakirai/mp4ff/mp4"
)

type keyRotationTransport func(*http.Request) (*http.Response, error)

func (f keyRotationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Exercise the real fragment producer without credentials, sockets, or a key
// server. Clear synthetic fragments isolate template selection from the cipher;
// the public embedded prefetch template supplies valid template JSON.
func runKeyRotation(t *testing.T, uris []*string, failURI string) (error, []string, string) {
	t.Helper()
	var input bytes.Buffer
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(48000, "audio", "eng")
	if err := init.Moov.Trak.SetAACDescriptor(2, 48000); err != nil {
		t.Fatal(err)
	}
	if err := init.Encode(&input); err != nil {
		t.Fatal(err)
	}
	segments := make([]*m3u8.MediaSegment, len(uris))
	for i, uri := range uris {
		frag, err := mp4.CreateFragment(uint32(i+1), 1)
		if err != nil {
			t.Fatal(err)
		}
		frag.AddFullSample(mp4.FullSample{
			Sample: mp4.Sample{Dur: 1024, Size: 4},
			Data:   []byte{0, 1, 2, 3},
		})
		if err := frag.Encode(&input); err != nil {
			t.Fatal(err)
		}
		segments[i] = &m3u8.MediaSegment{}
		if uri != nil {
			segments[i].Key = &m3u8.Key{URI: *uri}
		}
	}

	var calls []string
	original := download.Client
	download.Client = &http.Client{Transport: keyRotationTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "key-rotation.invalid" || req.URL.Path != "/key" {
			t.Errorf("unexpected request: %s", req.URL)
			return nil, errors.New("unexpected request")
		}
		uri := req.URL.Query().Get("uri")
		calls = append(calls, req.URL.Query().Get("adamId")+":"+uri)
		body := `{"code":0,"data":` + wrapper.PrefetchTemplateJSON + `}`
		if uri == failURI {
			body = `{"code":1,"msg":"synthetic template fetch failure"}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { download.Client = original })

	var cfg config.ConfigSet
	cfg.General.MaxMemoryLimit = 1
	out := filepath.Join(t.TempDir(), "output.m4a")
	err := downloadAndDecryptFile("https://key-rotation.invalid", bytes.NewReader(input.Bytes()), out,
		"123", segments, int64(input.Len()), cfg)
	return err, calls, out
}

func keyURI(uri string) *string { return &uri }

func TestKeyRotationSameURIReuse(t *testing.T) {
	err, calls, _ := runKeyRotation(t, []*string{keyURI("key-a"), keyURI("key-a"), keyURI("key-a")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"123:key-a"}) {
		t.Fatalf("same URI was refetched: %v", calls)
	}
}

func TestKeyRotationLateSwitchAndSwitchBackCache(t *testing.T) {
	err, calls, _ := runKeyRotation(t, []*string{
		keyURI("key-a"), keyURI("key-a"), keyURI("key-b"), keyURI("key-a"),
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"123:key-a", "123:key-b"}) {
		t.Fatalf("index 2 rotation or switch-back cache failed: %v", calls)
	}
}

func TestKeyRotationPrefetchIDZeroAndNormalAdamID(t *testing.T) {
	// wrapper.KeyJSON bypasses HTTP only for the combination of prefetch URI
	// and adam ID "0". Any other ID is recorded by our transport and fails the
	// assertion, even though it would receive valid JSON.
	err, calls, _ := runKeyRotation(t, []*string{
		keyURI(wrapper.PrefetchKey), keyURI("key-a"), keyURI(wrapper.PrefetchKey),
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"123:key-a"}) {
		t.Fatalf("prefetch ID must be 0 and ordinary ID must be 123: %v", calls)
	}
}

func TestKeyRotationFetchErrorDoesNotPublishStaleOrPlaintextOutput(t *testing.T) {
	for _, keys := range [][]*string{
		{keyURI("key-b")},
		{keyURI("key-a"), nil, keyURI("key-b")},
	} {
		err, calls, out := runKeyRotation(t, keys, "key-b")
		if err == nil {
			t.Fatalf("template fetch error was swallowed; calls=%v", calls)
		}
		if _, statErr := os.Stat(out); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("failed fetch published output: %v", statErr)
		}
	}
}

func TestKeyRotationNilKeyInheritance(t *testing.T) {
	for _, keys := range [][]*string{
		{nil, nil, nil},
		{keyURI("key-a"), nil, nil, keyURI("key-a")},
	} {
		err, calls, _ := runKeyRotation(t, keys, "")
		if err != nil {
			t.Fatal(err)
		}
		if keys[0] == nil {
			if len(calls) != 0 {
				t.Fatalf("initial nil key fetched a template: %v", calls)
			}
		} else if !reflect.DeepEqual(calls, []string{"123:key-a"}) {
			t.Fatalf("nil key did not preserve active template: %v", calls)
		}
	}
}

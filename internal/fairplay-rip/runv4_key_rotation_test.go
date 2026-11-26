package fairplayrip

import (
	"errors"
	"reflect"
	"testing"

	temarimod "github.com/WorldObservationLog/Temari/bindings/go"
	"github.com/grafov/m3u8"
)

// Only opaque Go pointers stand in for native templates. No library is loaded,
// no decryption is invoked and no network transport is used in these tests.
func TestTemplateSelectorKeyRotation(t *testing.T) {
	selector := newTemplateSelector()
	a, b := &temarimod.Temari{}, &temarimod.Temari{}
	var fetches []string
	fetch := func(adam, uri string) (*temarimod.Temari, error) {
		fetches = append(fetches, adam+":"+uri)
		if uri == "key-a" {
			return a, nil
		}
		if uri == "key-b" {
			return b, nil
		}
		t.Fatalf("unexpected URI %q", uri)
		return nil, nil
	}
	segments := []*m3u8.MediaSegment{
		{Key: &m3u8.Key{URI: "key-a"}},
		{Key: &m3u8.Key{URI: "key-a"}},
		{Key: &m3u8.Key{URI: "key-b"}}, // Rotation at index 2, not just the first two.
		{},                             // nil key inherits B.
		{Key: &m3u8.Key{}},             // empty URI inherits B.
		{Key: &m3u8.Key{URI: "key-a"}}, // Switch-back reuses cached A.
		{},                             // nil key now inherits A.
	}
	wantTemplates := []*temarimod.Temari{a, a, b, b, b, a, a}
	wantFetched := []bool{true, false, true, false, false, false, false}
	for i, segment := range segments {
		uri := ""
		if segment.Key != nil {
			uri = segment.Key.URI
		}
		got, fresh, err := selector.selectForKey(uri, "123", fetch)
		if err != nil || got != wantTemplates[i] || fresh != wantFetched[i] {
			t.Fatalf("segment %d: template=%p fresh=%v err=%v; want %p fresh=%v", i, got, fresh, err, wantTemplates[i], wantFetched[i])
		}
	}
	if !reflect.DeepEqual(fetches, []string{"123:key-a", "123:key-b"}) {
		t.Fatalf("fetches=%v", fetches)
	}
}

func TestTemplateSelectorPrefetch(t *testing.T) {
	selector := newTemplateSelector()
	prefetch, track := &temarimod.Temari{}, &temarimod.Temari{}
	var fetches []string
	fetch := func(adam, uri string) (*temarimod.Temari, error) {
		fetches = append(fetches, adam+":"+uri)
		if uri == prefetchKey {
			return prefetch, nil
		}
		return track, nil
	}
	for i, uri := range []string{prefetchKey, "key-a", prefetchKey} {
		got, fresh, err := selector.selectForKey(uri, "123", fetch)
		want := prefetch
		if i == 1 {
			want = track
		}
		if err != nil || got != want || fresh != (i < 2) {
			t.Fatalf("step %d: template=%p fresh=%v err=%v", i, got, fresh, err)
		}
	}
	if !reflect.DeepEqual(fetches, []string{"0:" + prefetchKey, "123:key-a"}) {
		t.Fatalf("prefetch must use Adam ID 0 exactly once: %v", fetches)
	}
}

func TestTemplateSelectorFetchErrorDoesNotReturnStaleTemplate(t *testing.T) {
	selector := newTemplateSelector()
	wantErr := errors.New("synthetic key fetch failure")
	a := &temarimod.Temari{}
	attempts := 0
	fetch := func(adam, uri string) (*temarimod.Temari, error) {
		if uri == "key-a" {
			return a, nil
		}
		attempts++
		return nil, wantErr
	}
	if got, fresh, err := selector.selectForKey("", "123", fetch); err != nil || fresh || got != nil {
		t.Fatalf("initial absent key: template=%p fresh=%v err=%v", got, fresh, err)
	}
	if _, _, err := selector.selectForKey("key-a", "123", fetch); err != nil {
		t.Fatal(err)
	}
	// A failed transition must return no template to the production caller,
	// which returns the error before enqueueing any fragment for this key.
	for i := 0; i < 2; i++ {
		got, fresh, err := selector.selectForKey("key-b", "123", fetch)
		if !errors.Is(err, wantErr) || got != nil || fresh {
			t.Fatalf("failed transition returned stale template: %p fresh=%v err=%v", got, fresh, err)
		}
		if selector.currentURI != "key-a" || len(selector.cache) != 1 || selector.cache["key-a"] != a {
			t.Fatalf("failed transition mutated selector: %+v", selector)
		}
	}
	if attempts != 2 {
		t.Fatalf("failed key was cached: attempts=%d", attempts)
	}
}

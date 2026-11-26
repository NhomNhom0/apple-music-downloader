package fairplayrip

import (
	"errors"
	"testing"

	temarimod "github.com/WorldObservationLog/Temari/bindings/go"
)

func TestTemariLibraryCandidates(t *testing.T) {
	tests := []struct {
		name string
		env  string
		exe  string
		err  error
		want []string
	}{
		{"override", "/override/libtemari.so", "/opt/amdl", nil, []string{"/override/libtemari.so"}},
		{"sibling then default", "", "/opt/bin/amdl", nil, []string{"/opt/bin/libtemari.so", ""}},
		{"default without executable", "", "", errors.New("unavailable"), []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := temariLibraryCandidates(func(string) string { return tt.env }, func() (string, error) { return tt.exe, tt.err })
			if len(got) != len(tt.want) {
				t.Fatalf("candidates = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("candidates = %#v, want %#v", got, tt.want)
				}
			}
		})
	}
}

func TestTemplateSelectorKeyRotation(t *testing.T) {
	selector := newTemplateSelector()
	fetches := []string{}
	fetch := func(adam, uri string) (*temarimod.Temari, error) {
		fetches = append(fetches, adam+":"+uri)
		return &temarimod.Temari{}, nil
	}

	first, fresh, err := selector.selectForKey("key-a", "123", fetch)
	if err != nil || !fresh || first == nil {
		t.Fatalf("first key: fresh=%v err=%v", fresh, err)
	}
	same, fresh, err := selector.selectForKey("key-a", "123", fetch)
	if err != nil || fresh || same != first {
		t.Fatalf("same key was fetched again")
	}
	inherited, fresh, err := selector.selectForKey("", "123", fetch)
	if err != nil || fresh || inherited != first {
		t.Fatalf("nil key did not retain active template")
	}

	// The selector has no segment-index gate: this represents a change at index 2.
	second, fresh, err := selector.selectForKey("key-b", "123", fetch)
	if err != nil || !fresh || second == first {
		t.Fatalf("changed key was not fetched")
	}
	switchedBack, fresh, err := selector.selectForKey("key-a", "123", fetch)
	if err != nil || fresh || switchedBack != first {
		t.Fatalf("switch-back did not reuse cache")
	}
	if len(fetches) != 2 || fetches[0] != "123:key-a" || fetches[1] != "123:key-b" {
		t.Fatalf("unexpected fetches: %#v", fetches)
	}
}

func TestTemplateSelectorPrefetchAndError(t *testing.T) {
	selector := newTemplateSelector()
	want := errors.New("key fetch failed")
	var adam string
	_, fresh, err := selector.selectForKey(prefetchKey, "123", func(got, uri string) (*temarimod.Temari, error) {
		adam = got
		return nil, want
	})
	if !errors.Is(err, want) || fresh || adam != "0" {
		t.Fatalf("prefetch error not propagated: adam=%q fresh=%v err=%v", adam, fresh, err)
	}
	if selector.currentURI != "" || len(selector.cache) != 0 {
		t.Fatalf("failed fetch mutated selector")
	}
}

package devices

import (
	"reflect"
	"testing"

	"github.com/pizzasaurusrex/homecast/internal/config"
	"github.com/pizzasaurusrex/homecast/internal/discovery"
)

func TestMerge_SavedOrderPreserved(t *testing.T) {
	saved := []config.Device{
		{ID: "b", Name: "Bravo", Enabled: true},
		{ID: "a", Name: "Alpha", Enabled: false},
	}
	got := Merge(saved, nil)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "b" || got[1].ID != "a" {
		t.Errorf("order not preserved: %+v", got)
	}
	for _, m := range got {
		if m.Discovered {
			t.Errorf("%s marked discovered with no mDNS data", m.ID)
		}
	}
}

func TestMerge_SavedTaggedByDiscovery(t *testing.T) {
	saved := []config.Device{{ID: "kitchen", Name: "Kitchen", Enabled: true}}
	disc := []discovery.Device{{ID: "kitchen", Name: "Kitchen", Addrs: []string{"10.0.0.5"}}}

	got := Merge(saved, disc)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if !got[0].Discovered {
		t.Error("saved + on LAN should mark Discovered=true")
	}
	if !reflect.DeepEqual(got[0].Addrs, []string{"10.0.0.5"}) {
		t.Errorf("Addrs = %v", got[0].Addrs)
	}
}

func TestMerge_DiscoveredOnlyAppendedDisabled(t *testing.T) {
	disc := []discovery.Device{
		{ID: "new", Name: "New Speaker", Addrs: []string{"10.0.0.6"}},
	}
	got := Merge(nil, disc)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Enabled {
		t.Error("discovered-only must default to Enabled=false")
	}
	if !got[0].Discovered {
		t.Error("discovered-only must have Discovered=true")
	}
}

func TestMerge_SavedNotSeenOnLAN(t *testing.T) {
	saved := []config.Device{{ID: "stale", Name: "Stale", Enabled: false}}
	got := Merge(saved, nil)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Discovered {
		t.Error("saved-but-absent must keep Discovered=false")
	}
	if got[0].Addrs != nil {
		t.Errorf("Addrs should be nil, got %v", got[0].Addrs)
	}
}

func TestMerge_SavedFirstThenDiscoveredOnly(t *testing.T) {
	saved := []config.Device{{ID: "saved", Name: "Saved", Enabled: true}}
	disc := []discovery.Device{
		{ID: "saved", Name: "Saved", Addrs: []string{"10.0.0.1"}},
		{ID: "fresh", Name: "Fresh", Addrs: []string{"10.0.0.2"}},
	}
	got := Merge(saved, disc)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	if got[0].ID != "saved" || got[1].ID != "fresh" {
		t.Errorf("order: %+v", got)
	}
}

func TestMerge_DropsMalformedDiscovered(t *testing.T) {
	disc := []discovery.Device{
		{ID: "", Name: "No ID"},
		{ID: "only-id", Name: ""},
		{ID: "good", Name: "Good"},
	}
	got := Merge(nil, disc)
	if len(got) != 1 || got[0].ID != "good" {
		t.Errorf("expected only 'good' to survive; got %+v", got)
	}
}

func TestMerge_DuplicateDiscoveredIDsCollapse(t *testing.T) {
	disc := []discovery.Device{
		{ID: "same", Name: "First", Addrs: []string{"10.0.0.1"}},
		{ID: "same", Name: "Second", Addrs: []string{"10.0.0.2"}},
	}
	got := Merge(nil, disc)
	if len(got) != 1 {
		t.Fatalf("duplicates not collapsed: %+v", got)
	}
	// First wins — avoids flapping as mDNS order changes.
	if got[0].Name != "First" {
		t.Errorf("expected first entry to win; got %+v", got[0])
	}
}

func TestMerge_AddrsAreCopied(t *testing.T) {
	addrs := []string{"10.0.0.1"}
	disc := []discovery.Device{{ID: "x", Name: "X", Addrs: addrs}}
	got := Merge(nil, disc)
	got[0].Addrs[0] = "mutated"
	if addrs[0] != "10.0.0.1" {
		t.Errorf("Merge must not alias caller's Addrs slice; addrs[0] = %q", addrs[0])
	}
}

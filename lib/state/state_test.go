package state_test

import (
	"path/filepath"
	"testing"

	"github.com/katbyte/acornvfd/lib/state"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "device.json")

	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load of missing file: %v", err)
	}
	if s.Address != "" || s.Device != "" {
		t.Fatalf("expected empty state, got %+v", s)
	}

	s.Remember("XGGF-1V48", "AA:BB:CC:DD:EE:FF")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	again, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if again.Device != "XGGF-1V48" || again.Address != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("round trip = %+v", again)
	}

	// a later connection with no advertised name keeps the last known name
	again.Remember("", "AA:BB:CC:DD:EE:00")
	if again.Device != "XGGF-1V48" || again.Address != "AA:BB:CC:DD:EE:00" {
		t.Fatalf("Remember with empty name = %+v", again)
	}
}

func TestLoadRejectsGarbage(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "device.json")
	if err := writeFile(path, "{not json"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Load(path); err == nil {
		t.Fatal("expected parse error")
	}
}

package store

import (
	"os"
	"path/filepath"
	"testing"

	"ipv6-proxy-manager/internal/model"
)

func TestSaveLoadAndBackupRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := New(path)
	state := model.NewState()
	state.PublicIPv4 = "203.0.113.5"
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	state.PublicIPv4 = "203.0.113.6"
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicIPv4 != "203.0.113.5" {
		t.Fatalf("expected backup recovery, got %q", got.PublicIPv4)
	}
}

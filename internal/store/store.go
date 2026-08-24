package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ipv6-proxy-manager/internal/model"
)

type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Store { return &Store{path: path} }

func (s *Store) Path() string { return s.path }

func (s *Store) Load() (model.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() (model.State, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.NewState(), nil
	}
	if err != nil {
		return model.State{}, fmt.Errorf("read state: %w", err)
	}
	var state model.State
	if err := json.Unmarshal(b, &state); err != nil {
		backup := s.path + ".backup"
		backupBytes, backupErr := os.ReadFile(backup)
		if backupErr != nil || json.Unmarshal(backupBytes, &state) != nil {
			return model.State{}, fmt.Errorf("state is corrupt and backup is unavailable: %w", err)
		}
	}
	if state.Version == 0 {
		state.Version = model.StateVersion
	}
	if state.NextPort == 0 {
		state.NextPort = 10000
	}
	if state.NextIPv6Offset == 0 {
		state.NextIPv6Offset = 1
	}
	if state.Proxies == nil {
		state.Proxies = []model.Proxy{}
	}
	return state, nil
}

func (s *Store) Save(state model.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(state)
}

func (s *Store) Update(fn func(*model.State) error) (model.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return model.State{}, err
	}
	if err := fn(&state); err != nil {
		return model.State{}, err
	}
	if err := s.saveLocked(state); err != nil {
		return model.State{}, err
	}
	return state, nil
}

func (s *Store) saveLocked(state model.State) error {
	state.Version = model.StateVersion
	state.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if old, err := os.ReadFile(s.path); err == nil {
		if err := writeAtomic(s.path+".backup", old, 0o600); err != nil {
			return fmt.Errorf("backup state: %w", err)
		}
	}
	if err := writeAtomic(s.path, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

func WriteAtomic(path string, data []byte, perm fs.FileMode) error {
	return writeAtomic(path, data, perm)
}

func writeAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		// Windows cannot replace an existing path atomically. Production Linux uses
		// the first branch; this fallback exists so development/tests remain portable.
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			return err
		}
		return os.Rename(tmpName, path)
	}
	return nil
}

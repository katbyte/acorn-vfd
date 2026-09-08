// Package state remembers the address of the last device acornvfd connected to. The clock advertises its name
// only intermittently, so name matching is unreliable; its address is stable per host, and reconnecting by the
// remembered address lets `acornvfd sync-time` and friends run without flags after the first connection.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// State is the on-disk document: the last device we connected to.
type State struct {
	Device  string `json:"device,omitempty"`
	Address string `json:"address,omitempty"`

	path string
}

// DefaultPath is <user config dir>/acornvfd/device.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	return filepath.Join(dir, "acornvfd", "device.json"), nil
}

// Load reads the state file at path ("" = DefaultPath). A missing file yields an empty State.
func Load(path string) (*State, error) {
	if path == "" {
		var err error
		if path, err = DefaultPath(); err != nil {
			return nil, err
		}
	}

	s := &State{path: path}

	b, err := os.ReadFile(path) //nolint:gosec // G304: the path is the user's own config file
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return s, nil
}

// Path returns where Save writes.
func (s *State) Path() string { return s.path }

// Remember records the device name and address, keeping the last known name when this connection had none (the
// clock frequently advertises with no name).
func (s *State) Remember(name, address string) {
	if name != "" {
		s.Device = name
	}
	s.Address = address
}

// Save writes the state file, creating the directory as needed.
func (s *State) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(s.path), err)
	}

	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}

	if err := os.WriteFile(s.path, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}

	return nil
}

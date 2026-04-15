// Package state persists the last resolved IP for each EC2 target, so
// `apply` can reuse it without calling AWS again.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	InstanceID string    `json:"instance_id"`
	PublicIP   string    `json:"public_ip"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type State struct {
	Targets map[string]Entry `json:"targets"` // keyed by target name
}

// Path returns the default cache file path (~/.cache/ec2hosts/state.json).
func Path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ec2hosts", "state.json"), nil
}

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{Targets: map[string]Entry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Targets == nil {
		s.Targets = map[string]Entry{}
	}
	return &s, nil
}

func (s *State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

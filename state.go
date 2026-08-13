package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type PersistentState struct {
	Working    []ProxyServer `json:"working"`
	Candidates []ProxyServer `json:"candidates"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type StateStore struct{ path string }

func NewStateStore(path string) *StateStore { return &StateStore{path: path} }

func (s *StateStore) Load() (PersistentState, error) {
	state := PersistentState{}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode state: %w", err)
	}
	return state, nil
}

func (s *StateStore) Save(state PersistentState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// idPattern mirrors internal/plugins/manifest's plugin id pattern:
// lowercase alphanumerics, dots, underscores, and hyphens, starting with a
// lowercase alphanumeric. manifest.Validate already enforces this shape
// upstream during install; this is the sink-level guard every FSStore
// method re-applies to id before building a filesystem path from it, so a
// malformed id (e.g. containing "..", "/", or "\") can never reach disk
// I/O regardless of how it got here.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// safePluginID rejects any id that is not a single, clean path segment
// matching idPattern. It is the guard called by every FSStore method
// before recordPath/configPath build a path from id.
func safePluginID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("plugins: invalid plugin id %q", id)
	}
	return nil
}

// FSStore persists plugin installation records under dir as "{id}.yml" (the
// Record) and "{id}.config.yml" (operator-editable config). FSStore
// implements Store.
type FSStore struct {
	dir string

	mu sync.Mutex
}

// NewFSStore returns a store that persists records under dir.
func NewFSStore(dir string) *FSStore {
	return &FSStore{dir: dir}
}

// recordPath returns the on-disk path for a plugin's installation record.
// Every public method validates id with safePluginID before reaching here.
func (s *FSStore) recordPath(id string) string {
	return filepath.Join(s.dir, id+".yml")
}

// configPath returns the on-disk path for a plugin's operator config.
func (s *FSStore) configPath(id string) string {
	return filepath.Join(s.dir, id+".config.yml")
}

// writeRecord marshals record to YAML and writes it to disk, creating the
// store directory if needed. Used both to create a fresh record (Install)
// and to persist updates to an existing one (Save).
func (s *FSStore) writeRecord(record *Record) error {
	if err := safePluginID(record.ID); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create plugin store dir: %w", err)
	}
	data, err := yaml.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal plugin record: %w", err)
	}
	if err := os.WriteFile(s.recordPath(record.ID), data, 0o600); err != nil {
		return fmt.Errorf("write plugin record: %w", err)
	}
	return nil
}

// Get loads the record for id from disk.
func (s *FSStore) Get(id string) (*Record, error) {
	if err := safePluginID(id); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.recordPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read plugin record: %w", err)
	}
	var record Record
	if err := yaml.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("unmarshal plugin record: %w", err)
	}
	return &record, nil
}

// List returns every installed plugin's record.
func (s *FSStore) List() ([]*Record, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugin store dir: %w", err)
	}

	var records []*Record
	for _, entry := range entries {
		if entry.IsDir() || !isRecordFile(entry.Name()) {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".yml")
		record, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// isRecordFile reports whether name is a plugin record file ("{id}.yml"),
// excluding config files ("{id}.config.yml").
func isRecordFile(name string) bool {
	return strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".config.yml")
}

// Save writes record to disk, creating it if it does not already exist or
// overwriting it (in full) if it does. Used both to persist a fresh install
// and to persist status/metadata updates to an existing record.
func (s *FSStore) Save(record *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeRecord(record)
}

// Delete removes the record and its operator config for id.
func (s *FSStore) Delete(id string) error {
	if err := safePluginID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.Get(id); err != nil {
		return err
	}

	if err := os.Remove(s.recordPath(id)); err != nil {
		return fmt.Errorf("delete plugin record: %w", err)
	}
	if err := os.Remove(s.configPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete plugin config: %w", err)
	}
	return nil
}

// GetConfig returns the operator-editable config for id, or an empty map if
// no config has been set yet.
func (s *FSStore) GetConfig(id string) (map[string]any, error) {
	if err := safePluginID(id); err != nil {
		return nil, err
	}
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.configPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read plugin config: %w", err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal plugin config: %w", err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, nil
}

// SetConfig replaces the operator-editable config for id.
func (s *FSStore) SetConfig(id string, config map[string]any) error {
	if err := safePluginID(id); err != nil {
		return err
	}
	if _, err := s.Get(id); err != nil {
		return err
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal plugin config: %w", err)
	}
	if err := os.WriteFile(s.configPath(id), data, 0o644); err != nil {
		return fmt.Errorf("write plugin config: %w", err)
	}
	return nil
}

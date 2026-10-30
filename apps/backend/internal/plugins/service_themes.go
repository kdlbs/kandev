package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/store"
)

const ThemeChangedEvent = "plugin.themes_changed"

type ThemeDescriptor struct {
	PackageID   string `json:"package_id"`
	PackageName string `json:"package_name"`
	Key         string `json:"key"`
	Label       string `json:"label"`
	Base        string `json:"base"`
	Version     string `json:"version"`
	Digest      string `json:"digest"`
}
type ThemeInventory struct {
	BootID   string            `json:"boot_id"`
	Revision uint64            `json:"revision"`
	Themes   []ThemeDescriptor `json:"themes"`
}
type themeEntry struct {
	descriptor ThemeDescriptor
	data       json.RawMessage
}
type themeState struct {
	mu       sync.RWMutex
	bootID   string
	revision uint64
	entries  map[string][]themeEntry
}

func loadThemeEntries(rec *store.Record) ([]themeEntry, error) {
	m, definitions, err := pkgtar.ReadThemeDirectory(rec.InstallPath)
	if err != nil {
		return nil, err
	}
	expected, _ := json.Marshal(rec.Manifest)
	actual, _ := json.Marshal(m)
	if string(expected) != string(actual) {
		return nil, fmt.Errorf("theme_manifest: stored manifest disagrees with package")
	}
	entries := make([]themeEntry, 0, len(m.Themes))
	for _, variant := range m.Themes {
		d := definitions[variant.Key]
		data, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		entries = append(entries, themeEntry{descriptor: ThemeDescriptor{PackageID: m.ID, PackageName: m.DisplayName, Key: variant.Key, Label: variant.Label, Base: variant.Base, Version: m.Version, Digest: d.Digest()}, data: data})
	}
	return entries, nil
}

func (s *Service) publishThemes(id string, entries []themeEntry) {
	state := &s.themes
	state.mu.Lock()
	if state.bootID == "" {
		state.bootID = uuid.NewString()
	}
	if state.entries == nil {
		state.entries = map[string][]themeEntry{}
	}
	if len(entries) == 0 {
		delete(state.entries, id)
	} else {
		state.entries[id] = entries
	}
	state.revision++
	event := map[string]any{"boot_id": state.bootID, "revision": state.revision}
	state.mu.Unlock()
	if s.eventBus != nil {
		_ = s.eventBus.Publish(context.Background(), ThemeChangedEvent, bus.NewEvent(ThemeChangedEvent, "plugins", event))
	}
}

func (s *Service) ThemeInventory() ThemeInventory {
	s.themes.mu.RLock()
	defer s.themes.mu.RUnlock()
	inventory := ThemeInventory{BootID: s.themes.bootID, Revision: s.themes.revision, Themes: []ThemeDescriptor{}}
	for _, entries := range s.themes.entries {
		for _, entry := range entries {
			inventory.Themes = append(inventory.Themes, entry.descriptor)
		}
	}
	sort.Slice(inventory.Themes, func(i, j int) bool {
		a, b := inventory.Themes[i], inventory.Themes[j]
		if a.PackageID == b.PackageID {
			return a.Key < b.Key
		}
		return a.PackageID < b.PackageID
	})
	return inventory
}

func (s *Service) ThemeContent(id, key, digest string) (json.RawMessage, error) {
	s.themes.mu.RLock()
	defer s.themes.mu.RUnlock()
	for _, entry := range s.themes.entries[id] {
		if entry.descriptor.Key == key && entry.descriptor.Digest == digest {
			return append(json.RawMessage(nil), entry.data...), nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *Service) installTheme(result *pkgtar.InstallResult, old *store.Record) (*store.Record, error) {
	if old != nil && !old.IsThemeDistribution() {
		_ = os.RemoveAll(result.InstallPath)
		return nil, fmt.Errorf("theme_profile: cannot replace an executable plugin")
	}
	rec := &store.Record{Manifest: *result.Manifest, Status: StatusActive, InstallPath: result.InstallPath, Signed: result.Signed, InstalledAt: time.Now().UTC(), InstallationID: uuid.NewString()}
	if old != nil {
		rec.InstallationID = old.InstallationID
		rec.AutoUpdate = old.AutoUpdate
	}
	entries, err := loadThemeEntries(rec)
	if err == nil {
		err = s.store.Save(rec)
	}
	if err != nil {
		_ = os.RemoveAll(result.InstallPath)
		return nil, err
	}
	s.registry.Add(rec)
	s.publishThemes(rec.ID, entries)
	s.pruneSupersededVersions(rec.ID, rec.Version, previousVersion(old, old != nil))
	return rec, nil
}

func (s *Service) activateTheme(rec *store.Record) error {
	entries, err := loadThemeEntries(rec)
	if err != nil {
		return err
	}
	if err = s.setStatus(rec.ID, StatusActive); err != nil {
		return err
	}
	s.publishThemes(rec.ID, entries)
	return nil
}

func (s *Service) restoreThemes() {
	for _, rec := range s.List() {
		if !rec.IsThemeDistribution() || rec.Status != StatusActive {
			continue
		}
		entries, err := loadThemeEntries(rec)
		if err != nil {
			_ = s.setStatusAndDiagnostic(rec.ID, StatusError, err, true)
			continue
		}
		s.publishThemes(rec.ID, entries)
	}
}

func (s *Service) uninstallTheme(rec *store.Record) error {
	// Persist revocation before deleting files, so failed removal stays recoverable.
	if rec.Status != StatusDisabled {
		if err := s.setStatus(rec.ID, StatusDisabled); err != nil {
			return err
		}
	}
	if err := pkgtar.Remove(s.pluginsDir, rec.ID); err != nil {
		return err
	}
	if err := s.store.Delete(rec.ID); err != nil {
		return err
	}
	s.registry.Remove(rec.ID)
	return nil
}

func (s *Service) installThemeProfile(result *pkgtar.InstallResult, old *store.Record, review *preparedTheme) (*store.Record, error, bool) {
	if review != nil && themePredecessor(old) != review.predecessor {
		_ = os.RemoveAll(result.InstallPath)
		return nil, ErrThemeReviewConflict, true
	}
	if result.Manifest.IsThemeDistribution() {
		rec, err := s.installTheme(result, old)
		return rec, err, true
	}
	if old != nil && old.IsThemeDistribution() {
		_ = os.RemoveAll(result.InstallPath)
		return nil, fmt.Errorf("theme_profile: cannot replace a theme with an executable plugin"), true
	}
	return nil, nil, false
}

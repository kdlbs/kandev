package plugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/plugins/themes"
	"gopkg.in/yaml.v3"
)

var ErrThemeReviewExpired = errors.New("theme_review_expired")
var ErrThemeReviewConflict = errors.New("theme_review_conflict")

type ThemeReview struct {
	ID          string                     `json:"id"`
	Manifest    manifest.Manifest          `json:"manifest"`
	Definitions map[string]json.RawMessage `json:"definitions"`
	Report      themes.Report              `json:"report"`
	ExpiresAt   time.Time                  `json:"expires_at"`
	Digest      string                     `json:"digest"`
}
type preparedTheme struct {
	review      ThemeReview
	owner       string
	expires     time.Time
	archive     []byte
	predecessor string
	entries     []themeEntry
	result      *store.Record
}
type themeReviewStore struct {
	mu      sync.Mutex
	entries map[string]*preparedTheme
}

func (s *Service) PrepareTheme(ctx context.Context, data []byte, base string) (*ThemeReview, error) {
	owner, err := themeAdminID(ctx)
	if err != nil {
		return nil, err
	}
	if len(data) > pkgtar.MaxThemePackageBytes {
		return nil, fmt.Errorf("theme_size: package limit exceeded")
	}
	archive, report, err := themeImportArchive(data, base)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "kandev-theme-review-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	installed, err := pkgtar.Install(bytes.NewReader(archive), root)
	if err != nil {
		return nil, err
	}
	if !installed.Manifest.IsThemeDistribution() {
		return nil, fmt.Errorf("theme_profile: theme package required")
	}
	if err = s.checkMinKandevVersion(installed.Manifest.MinKandevVersion); err != nil {
		return nil, err
	}
	entries, err := loadThemeEntries(&store.Record{Manifest: *installed.Manifest, InstallPath: installed.InstallPath})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sum := sha256.Sum256(archive)
	review := ThemeReview{ID: uuid.NewString(), Manifest: *installed.Manifest, Definitions: map[string]json.RawMessage{}, Report: report, ExpiresAt: now.Add(10 * time.Minute), Digest: hex.EncodeToString(sum[:])}
	review.Report.Fallback = 0
	for _, entry := range entries {
		review.Definitions[entry.descriptor.Key] = append(json.RawMessage(nil), entry.data...)
		definition, parseErr := themes.Parse(entry.data, entry.descriptor.Base)
		if parseErr != nil {
			return nil, parseErr
		}
		review.Report.Fallback += themes.CountFallback(definition)
		review.Report.Findings = append(review.Report.Findings, themes.ContrastFindings(definition)...)
	}
	old, _ := s.registry.Get(installed.Manifest.ID)
	prepared := &preparedTheme{review: review, owner: owner, expires: review.ExpiresAt, archive: archive, predecessor: themePredecessor(old), entries: entries}
	s.themeReviews.mu.Lock()
	defer s.themeReviews.mu.Unlock()
	if err = s.themeReviews.reserve(owner, len(archive), now); err != nil {
		return nil, err
	}
	s.themeReviews.entries[review.ID] = prepared
	return &review, nil
}

func (s *themeReviewStore) reserve(owner string, size int, now time.Time) error {
	if s.entries == nil {
		s.entries = map[string]*preparedTheme{}
	}
	count, total := 0, 0
	for id, review := range s.entries {
		if !review.expires.After(now) {
			delete(s.entries, id)
			continue
		}
		if review.owner == owner {
			count++
		}
		total += len(review.archive)
	}
	if count >= 8 || total+size > 64<<20 {
		return fmt.Errorf("theme_review_limit: too many pending imports")
	}
	return nil
}

func themeAdminID(ctx context.Context) (string, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || !identity.IsAdmin() || identity.UserID == "" {
		return "", fmt.Errorf("theme_admin_required")
	}
	return identity.UserID, nil
}

func themePredecessor(rec *store.Record) string {
	if rec == nil {
		return ""
	}
	data, _ := json.Marshal(rec)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Service) CommitTheme(ctx context.Context, id string) (*store.Record, error) {
	owner, err := themeAdminID(ctx)
	if err != nil {
		return nil, err
	}
	s.themeReviews.mu.Lock()
	defer s.themeReviews.mu.Unlock()
	review := s.themeReviews.entries[id]
	if review == nil || !review.expires.After(time.Now()) {
		delete(s.themeReviews.entries, id)
		return nil, ErrThemeReviewExpired
	}
	if review.owner != owner {
		return nil, fmt.Errorf("theme_review_owner: another principal owns this review")
	}
	if review.result != nil {
		return review.result, nil
	}
	// The predecessor is checked again under Install's existing lifecycle lock.
	rec, err := s.install(ctx, bytes.NewReader(review.archive), review)
	if errors.Is(err, pkgtar.ErrVersionExists) {
		rec, err = s.sameInstalledTheme(review)
	}
	if err != nil {
		return nil, err
	}
	review.result = rec
	return rec, nil
}

func themeImportArchive(data []byte, base string) ([]byte, themes.Report, error) {
	report := themes.Report{Findings: []themes.Finding{}}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		return append([]byte(nil), data...), report, nil
	}
	imported, err := themes.ConvertVSCode(data, base)
	if err != nil {
		return nil, report, err
	}
	definition, err := json.Marshal(imported.Definition)
	if err != nil {
		return nil, report, err
	}
	m := manifest.Manifest{ID: imported.ID, APIVersion: manifest.CurrentAPIVersion, Version: "1.0.0", DisplayName: imported.Name, Categories: []string{"theme"}, Distribution: &manifest.Distribution{SchemaVersion: 1, Kind: manifest.DistributionKindTheme}, Themes: []manifest.ThemeContribution{{Key: "default", Label: imported.Name, Base: imported.Definition.Base, Path: "theme.json"}}}
	manifestData, err := yaml.Marshal(m)
	if err != nil {
		return nil, report, err
	}
	var archive bytes.Buffer
	if err = pkgtartest.WritePackage(&archive, map[string][]byte{"manifest.yaml": manifestData, "theme.json": definition}); err != nil {
		return nil, report, err
	}
	return archive.Bytes(), imported.Report, nil
}

func (s *Service) sameInstalledTheme(review *preparedTheme) (*store.Record, error) {
	lock := s.lifecycleLocks.lockFor(review.review.Manifest.ID)
	lock.Lock()
	defer lock.Unlock()
	rec, ok := s.registry.Get(review.review.Manifest.ID)
	if !ok || themePredecessor(rec) != review.predecessor {
		return nil, ErrThemeReviewConflict
	}
	if !rec.IsThemeDistribution() || rec.Version != review.review.Manifest.Version {
		return nil, ErrThemeReviewConflict
	}
	entries, err := loadThemeEntries(rec)
	if err != nil {
		return nil, err
	}
	actual, _ := json.Marshal(rec.Manifest)
	expected, _ := json.Marshal(review.review.Manifest)
	if !bytes.Equal(actual, expected) || len(entries) != len(review.entries) {
		return nil, ErrThemeReviewConflict
	}
	for i, entry := range entries {
		if entry.descriptor.Digest != review.entries[i].descriptor.Digest {
			return nil, ErrThemeReviewConflict
		}
	}
	return rec, nil
}

// Package maintenance prepares local review artifacts through a closed file and
// test surface. It never launches a general worker or modifies the source repo.
package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
)

var ErrBoundary = errors.New("maintenance scope or validation boundary denied")

type Guard func(context.Context) error

type Sandbox struct {
	mu   sync.Mutex
	root string
	run  func(context.Context, string, string, ...string) (string, int, error)
}

func New(root string) *Sandbox { return &Sandbox{root: root} }

func (s *Sandbox) Prepare(ctx context.Context, source string, grant models.MaintenanceGrant, guard Guard) error {
	if guard == nil || grant.Scope.Validate() != nil || !filepath.IsAbs(source) || !objectID.MatchString(grant.BaseOID) {
		return ErrBoundary
	}
	if err := guard(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.jobPath(grant)
	if err != nil {
		return err
	}
	if _, err = os.Stat(job); err == nil {
		_, err = s.checkout(grant)
		return err
	}
	if err = os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	if err = os.Mkdir(job, 0700); err != nil {
		return err
	}
	dir := filepath.Join(job, "checkout")
	if err = os.Mkdir(dir, 0700); err != nil {
		return err
	}
	commands := [][]string{{"init", "-q"}, {"-c", "protocol.file.allow=always", "fetch", "--quiet", "--depth=1", "--no-tags", "--", source, grant.BaseOID}, {"checkout", "--quiet", "--detach", "FETCH_HEAD"}}
	for _, args := range commands {
		if err = guard(ctx); err != nil {
			return err
		}
		if _, err = runGit(ctx, dir, args...); err != nil {
			return err
		}
	}
	return writePrivateJSON(filepath.Join(job, "manifest.json"), sandboxIdentity(grant))
}

func (s *Sandbox) Read(ctx context.Context, grant models.MaintenanceGrant, path string) (models.MaintenanceFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(ctx, grant, path)
}

func (s *Sandbox) read(ctx context.Context, grant models.MaintenanceGrant, path string) (models.MaintenanceFile, error) {
	if err := ctx.Err(); err != nil {
		return models.MaintenanceFile{}, err
	}
	if !slices.Contains(grant.Scope.Actions, "read") || !slices.Contains(grant.Scope.Files, path) || !models.MaintenanceFileAllowed(path) {
		return models.MaintenanceFile{}, ErrBoundary
	}
	dir, err := s.checkout(grant)
	if err != nil {
		return models.MaintenanceFile{}, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return models.MaintenanceFile{}, err
	}
	defer func() { _ = root.Close() }()
	if err = regularPath(root, path); err != nil {
		return models.MaintenanceFile{}, err
	}
	file, err := root.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return models.MaintenanceFile{Path: path, SHA256: "missing"}, nil
	}
	if err != nil {
		return models.MaintenanceFile{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 32768 {
		return models.MaintenanceFile{}, ErrBoundary
	}
	return readMaintenanceFile(root, path)
}

func readMaintenanceFile(root *os.Root, path string) (models.MaintenanceFile, error) {
	data, err := root.ReadFile(path)
	if err != nil {
		return models.MaintenanceFile{}, err
	}
	if len(data) > 32768 || !utf8.Valid(data) {
		return models.MaintenanceFile{}, ErrBoundary
	}
	return models.MaintenanceFile{Path: path, Content: string(data), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}, nil
}

func (s *Sandbox) Patch(ctx context.Context, grant models.MaintenanceGrant, file models.MaintenanceFile, guard Guard) (models.MaintenanceArtifact, error) {
	if guard == nil || !slices.Contains(grant.Scope.Actions, "patch") || len(file.Content) > 32768 || !utf8.ValidString(file.Content) || strings.ContainsRune(file.Content, 0) {
		return models.MaintenanceArtifact{}, ErrBoundary
	}
	if err := guard(ctx); err != nil {
		return models.MaintenanceArtifact{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.read(ctx, grant, file.Path)
	if err != nil {
		return models.MaintenanceArtifact{}, err
	}
	if current.SHA256 != file.SHA256 {
		return models.MaintenanceArtifact{}, models.ErrConflict
	}
	dir, err := s.checkout(grant)
	if err != nil {
		return models.MaintenanceArtifact{}, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return models.MaintenanceArtifact{}, err
	}
	defer func() { _ = root.Close() }()
	if err = guard(ctx); err != nil {
		return models.MaintenanceArtifact{}, err
	}
	if err = regularPath(root, file.Path); err != nil {
		return models.MaintenanceArtifact{}, err
	}
	if err = root.MkdirAll(filepath.Dir(file.Path), 0700); err != nil {
		return models.MaintenanceArtifact{}, err
	}
	if err = root.WriteFile(file.Path, []byte(file.Content), 0600); err != nil {
		return models.MaintenanceArtifact{}, err
	}
	tree, err := stageTree(ctx, dir, grant.Scope.Files)
	return models.MaintenanceArtifact{BaseOID: grant.BaseOID, TreeOID: tree}, err
}

var objectID = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)

func (s *Sandbox) jobPath(grant models.MaintenanceGrant) (string, error) {
	if _, err := uuid.Parse(grant.CandidateID); err != nil || !filepath.IsAbs(s.root) {
		return "", ErrBoundary
	}
	return filepath.Join(s.root, grant.CandidateID), nil
}

func sandboxIdentity(grant models.MaintenanceGrant) string {
	raw, _ := json.Marshal([]any{grant.CandidateID, grant.BindingID, grant.OwnerUserID, grant.BaseOID, grant.Scope})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func (s *Sandbox) checkout(grant models.MaintenanceGrant) (string, error) {
	job, err := s.jobPath(grant)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(job, "manifest.json"))
	var identity string
	if err != nil || json.Unmarshal(raw, &identity) != nil || identity != sandboxIdentity(grant) {
		return "", ErrBoundary
	}
	dir := filepath.Join(job, "checkout")
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil || canonical != dir {
		return "", ErrBoundary
	}
	return dir, nil
}

func regularPath(root *os.Root, path string) error {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrBoundary
		}
	}
	return nil
}

func writePrivateJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = os.WriteFile(path+".tmp", raw, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

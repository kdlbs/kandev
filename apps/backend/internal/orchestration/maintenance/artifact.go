package maintenance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/orchestration/models"
)

func (s *Sandbox) Review(ctx context.Context, grant models.MaintenanceGrant) (models.MaintenanceReviewArtifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := models.MaintenanceReviewArtifact{MaintenanceArtifact: models.MaintenanceArtifact{BaseOID: grant.BaseOID}}
	dir, err := s.checkout(grant)
	if err != nil {
		return result, err
	}
	result.CommitOID, err = runGit(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return result, err
	}
	result.TreeOID, err = runGit(ctx, dir, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return result, err
	}
	result.Patch, err = reviewPatch(ctx, dir, grant, result.CommitOID)
	if err != nil {
		return result, err
	}
	result.PatchSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(result.Patch)))
	job, _ := s.jobPath(grant)
	raw, err := os.ReadFile(filepath.Join(job, "validation.json"))
	if os.IsNotExist(err) {
		return result, nil
	}
	var validation models.MaintenanceValidation
	if err != nil || json.Unmarshal(raw, &validation) != nil {
		return result, ErrBoundary
	}
	if validation.TreeOID == result.TreeOID {
		result.Validation = &validation
	}
	return result, nil
}

func reviewPatch(ctx context.Context, dir string, grant models.MaintenanceGrant, head string) (string, error) {
	paths, err := runGit(ctx, dir, "diff", "--no-ext-diff", "--no-textconv", "--name-only", "-z", grant.BaseOID, head, "--")
	if err != nil {
		return "", err
	}
	for _, path := range strings.Split(paths, "\x00") {
		if path != "" && (!slices.Contains(grant.Scope.Files, path) || !models.MaintenanceFileAllowed(path)) {
			return "", ErrBoundary
		}
	}
	patch, err := runGit(ctx, dir, "diff", "--no-ext-diff", "--no-textconv", "--binary", "--src-prefix=a/", "--dst-prefix=b/", grant.BaseOID, head, "--")
	if patch != "" {
		patch += "\n"
	}
	return patch, err
}

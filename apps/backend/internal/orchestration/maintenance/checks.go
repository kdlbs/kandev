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

	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (s *Sandbox) Inspect(ctx context.Context, source string, scope models.MaintenanceScope) (models.MaintenanceScope, string, error) {
	if scope.Validate() != nil || !filepath.IsAbs(source) || os.Geteuid() == 0 || strings.ContainsAny(s.root, ",\r\n") {
		return scope, "", ErrBoundary
	}
	base, err := runGit(ctx, source, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || !objectID.MatchString(base) {
		return scope, "", ErrBoundary
	}
	image, _, err := s.docker(ctx, "image", "inspect", "--format", "{{.Id}} {{.Os}}", "--", scope.Image)
	parts := strings.Fields(image)
	if err != nil || len(parts) != 2 || parts[1] != "linux" || !strings.HasPrefix(parts[0], "sha256:") || !objectID.MatchString(strings.TrimPrefix(parts[0], "sha256:")) {
		return scope, "", fmt.Errorf("maintenance requires an available local Linux container image")
	}
	scope.Image = parts[0]
	if err = s.qualifySandbox(ctx, scope.Image); err != nil {
		return scope, "", err
	}
	return scope, base, nil
}

func (s *Sandbox) Check(ctx context.Context, grant models.MaintenanceGrant, guard Guard) (models.MaintenanceValidation, error) {
	result := models.MaintenanceValidation{GrantRevision: grant.Revision, Passed: true, Checks: []models.MaintenanceCheck{}}
	if guard == nil || !slices.Contains(grant.Scope.Actions, "test") {
		return result, ErrBoundary
	}
	if err := guard(ctx); err != nil {
		return result, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.checkout(grant)
	if err != nil {
		return result, err
	}
	result.TreeOID, err = stageTree(ctx, dir, grant.Scope.Files)
	if err != nil {
		return result, err
	}
	for i, argv := range [][]string{grant.Scope.Positive, grant.Scope.Negative} {
		if err = guard(ctx); err != nil {
			return result, err
		}
		out, code, runErr := s.containerCheck(ctx, dir, grant.Scope.Image, argv, guard)
		if runErr != nil {
			return result, runErr
		}
		kind := "positive"
		if i == 1 {
			kind = "negative"
		}
		safe := []rune(redaction.NewRedactor().String(out))
		result.Checks = append(result.Checks, models.MaintenanceCheck{Kind: kind, ExitCode: code, OutputHash: fmt.Sprintf("%x", sha256.Sum256([]byte(out))), Summary: string(safe[:min(len(safe), 1200)])})
		result.Passed = result.Passed && code == 0
	}
	current, err := stageTree(ctx, dir, grant.Scope.Files)
	if err != nil || current != result.TreeOID {
		return result, ErrBoundary
	}
	job, _ := s.jobPath(grant)
	return result, writePrivateJSON(filepath.Join(job, "validation.json"), result)
}

func (s *Sandbox) Commit(ctx context.Context, grant models.MaintenanceGrant, guard Guard) (models.MaintenanceArtifact, error) {
	artifact := models.MaintenanceArtifact{BaseOID: grant.BaseOID}
	if guard == nil || !slices.Contains(grant.Scope.Actions, "commit") {
		return artifact, ErrBoundary
	}
	if err := guard(ctx); err != nil {
		return artifact, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.checkout(grant)
	if err != nil {
		return artifact, err
	}
	artifact.TreeOID, err = s.validatedTree(ctx, dir, grant)
	if err != nil {
		return artifact, err
	}
	head, err := runGit(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return artifact, err
	}
	previous, err := runGit(ctx, dir, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return artifact, err
	}
	if previous == artifact.TreeOID && head != grant.BaseOID {
		artifact.CommitOID = head
		return artifact, nil
	}
	if previous == artifact.TreeOID {
		return artifact, fmt.Errorf("maintenance requires a changed, validated tree")
	}
	if err = guard(ctx); err != nil {
		return artifact, err
	}
	if _, err = runGit(ctx, dir, "commit", "--quiet", "--no-verify", "-m", "Prepare scoped workflow repair"); err != nil {
		return artifact, err
	}
	artifact.CommitOID, err = runGit(ctx, dir, "rev-parse", "HEAD")
	return artifact, err
}

func (s *Sandbox) validatedTree(ctx context.Context, dir string, grant models.MaintenanceGrant) (string, error) {
	job, _ := s.jobPath(grant)
	raw, err := os.ReadFile(filepath.Join(job, "validation.json"))
	var validation models.MaintenanceValidation
	if err != nil || json.Unmarshal(raw, &validation) != nil || !validation.Passed || validation.GrantRevision != grant.Revision || len(validation.Checks) != 2 {
		return "", ErrBoundary
	}
	tree, err := stageTree(ctx, dir, grant.Scope.Files)
	if err != nil || tree != validation.TreeOID {
		return "", ErrBoundary
	}
	return tree, nil
}

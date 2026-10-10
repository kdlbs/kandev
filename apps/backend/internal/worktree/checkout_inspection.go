package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/subproc"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

type checkoutClass uint8

const (
	checkoutAmbiguous checkoutClass = iota
	checkoutLinkedHealthy
	checkoutMainHealthy
	checkoutLinkedMissingAdmin
)

type checkoutInspection struct {
	class          checkoutClass
	linked         linkedWorktreeInspection
	reason         string
	operationalErr error
}

type mainCheckoutGitCheck struct {
	args   []string
	reason string
	valid  func(string) bool
}

// inspectCheckout adds main-repository recognition at the admission and reuse
// boundaries while retaining the linked-worktree inspector for pointer files.
func (m *Manager) inspectCheckout(
	ctx context.Context,
	path string,
	handle storageworkspaces.DirectoryHandle,
) checkoutInspection {
	if err := ctx.Err(); err != nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: err.Error(), operationalErr: err}
	}
	if handle == nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: "checkout directory is not pinned"}
	}
	if err := handle.VerifyPath(filepath.Clean(path)); err != nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: fmt.Sprintf("checkout path changed during inspection: %v", err)}
	}
	gitMode, err := handle.LstatEntry(".git")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return checkoutInspection{class: checkoutAmbiguous, reason: "checkout Git metadata is missing"}
		}
		return checkoutInspection{class: checkoutAmbiguous, reason: fmt.Sprintf("cannot inspect checkout Git metadata: %v", err)}
	}
	if gitMode&os.ModeSymlink != 0 {
		return checkoutInspection{class: checkoutAmbiguous, reason: "checkout Git metadata is a symbolic link"}
	}
	if gitMode.IsRegular() {
		if !handle.IsValidWorktree() {
			return checkoutInspection{class: checkoutAmbiguous, reason: "checkout Git pointer is malformed"}
		}
		linked := inspectLinkedWorktree(path)
		switch linked.class {
		case linkedWorktreeHealthy:
			return checkoutInspection{class: checkoutLinkedHealthy, linked: linked}
		case linkedWorktreeMissingAdmin:
			return checkoutInspection{class: checkoutLinkedMissingAdmin, linked: linked}
		default:
			return checkoutInspection{class: checkoutAmbiguous, linked: linked, reason: linked.reason}
		}
	}
	if !gitMode.IsDir() {
		return checkoutInspection{class: checkoutAmbiguous, reason: "checkout Git metadata is not a file or directory"}
	}
	return m.inspectMainCheckout(ctx, path, handle)
}

func (m *Manager) inspectMainCheckout(
	ctx context.Context,
	path string,
	checkoutHandle storageworkspaces.DirectoryHandle,
) checkoutInspection {
	metadataHandle, err := checkoutHandle.OpenSubdirectory(".git")
	if err != nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: fmt.Sprintf("cannot pin main-repository Git metadata: %v", err)}
	}
	defer func() { _ = metadataHandle.Close() }()

	cleanPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: "cannot resolve main-repository checkout path"}
	}
	gitDir := filepath.Join(cleanPath, ".git")
	if err := metadataHandle.VerifyPath(gitDir); err != nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: "main-repository Git metadata changed during inspection"}
	}
	if reason := validateMainGitMetadataLayout(metadataHandle); reason != "" {
		return checkoutInspection{class: checkoutAmbiguous, reason: reason}
	}

	if inspection, failed := m.inspectMainCheckoutGit(ctx, cleanPath, gitDir); failed {
		return inspection
	}
	if inspection := m.inspectMainCheckoutHead(ctx, cleanPath, gitDir); inspection.class != checkoutMainHealthy {
		return inspection
	}
	if err := ctx.Err(); err != nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: err.Error(), operationalErr: err}
	}
	if err := metadataHandle.VerifyPath(gitDir); err != nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: "main-repository Git metadata changed during inspection"}
	}
	if err := checkoutHandle.VerifyPath(cleanPath); err != nil {
		return checkoutInspection{class: checkoutAmbiguous, reason: "main-repository checkout changed during inspection"}
	}
	return checkoutInspection{class: checkoutMainHealthy}
}

func (m *Manager) inspectMainCheckoutGit(ctx context.Context, checkoutPath, gitDir string) (checkoutInspection, bool) {
	checks := []mainCheckoutGitCheck{
		{
			args:   []string{"rev-parse", "--show-toplevel"},
			reason: "Git metadata does not resolve to the selected checkout",
			valid: func(output string) bool {
				return sameDirectoryIdentity(checkoutPath, strings.TrimSpace(output))
			},
		},
		{
			args:   []string{"--work-tree=" + checkoutPath, "rev-parse", "--show-toplevel"},
			reason: "explicit Git work-tree inspection resolved a different checkout",
			valid: func(output string) bool {
				return sameDirectoryIdentity(checkoutPath, strings.TrimSpace(output))
			},
		},
		{
			args:   []string{"rev-parse", "--absolute-git-dir"},
			reason: "Git metadata resolves to a different directory",
			valid: func(output string) bool {
				return sameDirectoryIdentity(gitDir, strings.TrimSpace(output))
			},
		},
		{
			args:   []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
			reason: "Git common directory is redirected",
			valid: func(output string) bool {
				return sameDirectoryIdentity(gitDir, strings.TrimSpace(output))
			},
		},
		{
			args:   []string{"rev-parse", "--is-bare-repository"},
			reason: "Git metadata does not describe a non-bare repository",
			valid:  func(output string) bool { return strings.TrimSpace(output) == "false" },
		},
		{
			args:   []string{"--work-tree=" + checkoutPath, "rev-parse", "--is-inside-work-tree"},
			reason: "Git metadata does not describe a working tree",
			valid:  func(output string) bool { return strings.TrimSpace(output) == "true" },
		},
	}
	for _, check := range checks {
		output, err := m.runMainCheckoutGit(ctx, checkoutPath, gitDir, check.args...)
		if err != nil || !check.valid(output) {
			return mainCheckoutInspectionFailure(ctx, check.reason, err), true
		}
	}
	return checkoutInspection{}, false
}

func (m *Manager) inspectMainCheckoutHead(ctx context.Context, checkoutPath, gitDir string) checkoutInspection {
	if ref, err := m.runMainCheckoutGit(ctx, checkoutPath, gitDir, "symbolic-ref", "--quiet", "HEAD"); err == nil {
		ref = strings.TrimSpace(ref)
		if !strings.HasPrefix(ref, "refs/heads/") || strings.TrimPrefix(ref, "refs/heads/") == "" {
			return checkoutInspection{class: checkoutAmbiguous, reason: "Git HEAD does not reference a local branch"}
		}
		if _, err := m.runMainCheckoutGit(ctx, checkoutPath, gitDir, "check-ref-format", ref); err != nil {
			return mainCheckoutInspectionFailure(ctx, "Git HEAD references an invalid local branch", err)
		}
		return checkoutInspection{class: checkoutMainHealthy}
	} else if operationalErr := checkoutInspectionOperationalError(ctx, err); operationalErr != nil {
		return checkoutInspection{
			class: checkoutAmbiguous, reason: "Git HEAD inspection could not complete", operationalErr: operationalErr,
		}
	}
	if _, err := m.runMainCheckoutGit(ctx, checkoutPath, gitDir, "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
		return mainCheckoutInspectionFailure(ctx, "Git HEAD is neither a valid branch reference nor a commit", err)
	}
	return checkoutInspection{class: checkoutMainHealthy}
}

func mainCheckoutInspectionFailure(ctx context.Context, reason string, commandErr error) checkoutInspection {
	return checkoutInspection{
		class: checkoutAmbiguous, reason: reason,
		operationalErr: checkoutInspectionOperationalError(ctx, commandErr),
	}
}

func checkoutInspectionOperationalError(ctx context.Context, err error) error {
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func (m *Manager) runMainCheckoutGit(ctx context.Context, checkoutPath, gitDir string, args ...string) (string, error) {
	explicit := []string{"--git-dir=" + gitDir}
	explicit = append(explicit, args...)
	return m.runBoundedGitInspectWithEnvironment(ctx, checkoutPath, mainCheckoutInspectionEnvironment, explicit...)
}

func mainCheckoutInspectionEnvironment(env []string) []string {
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			continue
		}
		clean = append(clean, entry)
	}
	return subproc.PrepareGitEnvironment(clean)
}

func validateMainGitMetadataLayout(handle storageworkspaces.DirectoryHandle) string {
	for _, entry := range []struct {
		name      string
		directory bool
		optional  bool
	}{
		{name: "HEAD"},
		{name: "config"},
		{name: "objects", directory: true},
		{name: "index", optional: true},
		{name: "packed-refs", optional: true},
	} {
		mode, err := handle.LstatEntry(entry.name)
		if errors.Is(err, os.ErrNotExist) && entry.optional {
			continue
		}
		if err != nil {
			return fmt.Sprintf("cannot inspect main-repository %s metadata", entry.name)
		}
		if mode&os.ModeSymlink != 0 {
			return fmt.Sprintf("main-repository %s metadata is a symbolic link", entry.name)
		}
		if entry.directory && !mode.IsDir() {
			return fmt.Sprintf("main-repository %s metadata is not a directory", entry.name)
		}
		if !entry.directory && !mode.IsRegular() {
			return fmt.Sprintf("main-repository %s metadata is not a regular file", entry.name)
		}
	}
	if _, err := handle.LstatEntry("commondir"); err == nil {
		return "main-repository Git metadata contains a common-directory redirect"
	} else if !errors.Is(err, os.ErrNotExist) {
		return "cannot inspect main-repository common-directory metadata"
	}
	return ""
}

func sameDirectoryIdentity(expected, actual string) bool {
	if expected == "" || actual == "" {
		return false
	}
	expectedInfo, err := os.Lstat(expected)
	if err != nil || !expectedInfo.IsDir() || expectedInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	actualInfo, err := os.Lstat(actual)
	if err != nil || !actualInfo.IsDir() || actualInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return os.SameFile(expectedInfo, actualInfo)
}

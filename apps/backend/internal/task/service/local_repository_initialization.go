package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

var (
	ErrInvalidLocalRepositoryInitialization = errors.New("invalid local repository initialization")
	ErrLocalRepositoryTargetExists          = errors.New("local repository target exists")
	errLocalRepositoryTargetChanged         = errors.New("local repository target changed during initialization")
)

type initializeGitRepositoryFunc func(context.Context, string, *os.File) error

type localRepositoryStaging struct {
	path      string
	directory *os.File
	identity  fs.FileInfo
}

// InitializeLocalRepository creates an empty Git repository and registers it with a workspace.
func (s *Service) InitializeLocalRepository(
	ctx context.Context,
	req *InitializeLocalRepositoryRequest,
) (*models.Repository, error) {
	return s.initializeLocalRepository(ctx, req, initializeGitRepository)
}

func (s *Service) initializeLocalRepository(
	ctx context.Context,
	req *InitializeLocalRepositoryRequest,
	initializeGit initializeGitRepositoryFunc,
) (*models.Repository, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	if err := validateLocalRepositoryName(name); err != nil {
		return nil, err
	}
	parentPath, err := canonicalLocalRepositoryParent(req.ParentPath)
	if err != nil {
		return nil, err
	}
	targetPath := filepath.Join(parentPath, name)
	if _, statErr := os.Lstat(targetPath); statErr == nil {
		return nil, fmt.Errorf("%w: %s", ErrLocalRepositoryTargetExists, targetPath)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: target path cannot be inspected", ErrInvalidLocalRepositoryInitialization)
	}

	staging, err := createLocalRepositoryStaging(parentPath)
	if err != nil {
		return nil, err
	}
	defer s.closeLocalRepositoryStaging(staging)
	published := false
	cleanup := func() {
		cleanupPath := staging.path
		if published {
			cleanupPath = targetPath
		}
		s.cleanupInitializedLocalRepository(cleanupPath, staging.directory, staging.identity)
	}

	if initErr := initializeGit(ctx, staging.path, staging.directory); initErr != nil {
		cleanup()
		return nil, fmt.Errorf("initialize git repository: %w", initErr)
	}
	if !localRepositoryTargetMatches(staging.path, staging.identity) {
		cleanup()
		return nil, errLocalRepositoryTargetChanged
	}
	if publishErr := publishLocalRepository(staging.path, targetPath); publishErr != nil {
		cleanup()
		if errors.Is(publishErr, fs.ErrExist) {
			return nil, fmt.Errorf("%w: %s", ErrLocalRepositoryTargetExists, targetPath)
		}
		return nil, fmt.Errorf("publish initialized local repository: %w", publishErr)
	}
	published = true
	if chmodErr := staging.directory.Chmod(0o755); chmodErr != nil {
		cleanup()
		return nil, fmt.Errorf("set initialized local repository permissions: %w", chmodErr)
	}
	if !localRepositoryTargetMatches(targetPath, staging.identity) {
		cleanup()
		return nil, errLocalRepositoryTargetChanged
	}

	repository, err := s.createRepositoryWithCanonicalPath(ctx, &CreateRepositoryRequest{
		WorkspaceID:   req.WorkspaceID,
		Name:          name,
		SourceType:    sourceTypeLocal,
		LocalPath:     targetPath,
		DefaultBranch: "main",
	})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("persist initialized local repository: %w", err)
	}
	return repository, nil
}

func createLocalRepositoryStaging(parentPath string) (*localRepositoryStaging, error) {
	stagingPath, err := os.MkdirTemp(parentPath, ".kandev-repository-init-")
	if err != nil {
		return nil, fmt.Errorf("%w: parent directory is not writable", ErrInvalidLocalRepositoryInitialization)
	}
	stagingPathInfo, err := os.Lstat(stagingPath)
	if err != nil || !stagingPathInfo.IsDir() || stagingPathInfo.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(stagingPath)
		return nil, errLocalRepositoryTargetChanged
	}
	stagingDirectory, err := openLocalRepositoryDirectory(stagingPath)
	if err != nil {
		_ = os.Remove(stagingPath)
		return nil, fmt.Errorf("open local repository staging directory: %w", err)
	}
	stagingIdentity, err := stagingDirectory.Stat()
	if err != nil || !os.SameFile(stagingPathInfo, stagingIdentity) ||
		!localRepositoryTargetMatches(stagingPath, stagingIdentity) ||
		!localRepositoryDirectoryOwnedByProcess(stagingIdentity) || stagingIdentity.Mode().Perm() != 0o700 {
		_ = stagingDirectory.Close()
		_ = os.Remove(stagingPath)
		return nil, errLocalRepositoryTargetChanged
	}
	if entries, readErr := stagingDirectory.ReadDir(1); len(entries) != 0 || !errors.Is(readErr, io.EOF) {
		_ = stagingDirectory.Close()
		_ = os.Remove(stagingPath)
		return nil, errLocalRepositoryTargetChanged
	}
	return &localRepositoryStaging{
		path:      stagingPath,
		directory: stagingDirectory,
		identity:  stagingIdentity,
	}, nil
}

func (s *Service) closeLocalRepositoryStaging(staging *localRepositoryStaging) {
	if err := staging.directory.Close(); err != nil {
		s.logger.Warn("failed to close local repository staging directory",
			zap.String("path", staging.path), zap.Error(err))
	}
}

func initializeGitRepository(ctx context.Context, targetPath string, targetDirectory *os.File) error {
	gitTarget := targetPath
	var extraFiles []*os.File
	if inheritedPath := inheritedDirectoryPath(3); inheritedPath != "" {
		gitTarget = inheritedPath
		extraFiles = []*os.File{targetDirectory}
	}
	command := exec.CommandContext(ctx, "git", "init", "--initial-branch=main", gitTarget)
	command.ExtraFiles = extraFiles
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *Service) cleanupInitializedLocalRepository(
	targetPath string,
	targetDirectory *os.File,
	targetIdentity fs.FileInfo,
) {
	if anchoredPath := inheritedDirectoryPath(int(targetDirectory.Fd())); anchoredPath != "" {
		if err := os.RemoveAll(filepath.Join(anchoredPath, ".git")); err != nil {
			s.logger.Warn("failed to clean up initialized Git metadata",
				zap.String("path", targetPath), zap.Error(err))
		}
	} else if localRepositoryTargetMatches(targetPath, targetIdentity) {
		if err := os.RemoveAll(filepath.Join(targetPath, ".git")); err != nil {
			s.logger.Warn("failed to clean up initialized Git metadata",
				zap.String("path", targetPath), zap.Error(err))
		}
	}
	if !localRepositoryTargetMatches(targetPath, targetIdentity) {
		s.logger.Warn("refusing to remove replaced local repository directory",
			zap.String("path", targetPath))
		return
	}
	if err := os.Remove(targetPath); err != nil {
		s.logger.Warn("failed to remove initialized local repository directory",
			zap.String("path", targetPath), zap.Error(err))
	}
}

func inheritedDirectoryPath(fd int) string {
	switch runtime.GOOS {
	case "linux":
		return "/proc/self/fd/" + strconv.Itoa(fd)
	case "darwin", "freebsd", "openbsd", "netbsd":
		return "/dev/fd/" + strconv.Itoa(fd)
	default:
		return ""
	}
}

func localRepositoryTargetMatches(targetPath string, createdTarget fs.FileInfo) bool {
	currentTarget, err := os.Lstat(targetPath)
	return err == nil && currentTarget.IsDir() && os.SameFile(createdTarget, currentTarget)
}

func validateLocalRepositoryName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return fmt.Errorf("%w: name must be a single path segment", ErrInvalidLocalRepositoryInitialization)
	}
	return nil
}

func canonicalLocalRepositoryParent(parentPath string) (string, error) {
	if parentPath == "" || !filepath.IsAbs(parentPath) {
		return "", fmt.Errorf("%w: parent_path must be absolute", ErrInvalidLocalRepositoryInitialization)
	}
	canonicalPath, err := filepath.EvalSymlinks(filepath.Clean(parentPath))
	if err != nil {
		return "", fmt.Errorf("%w: parent directory cannot be accessed", ErrInvalidLocalRepositoryInitialization)
	}
	// codeql[go/path-injection] The selected absolute parent is canonicalized before inspection.
	info, err := os.Stat(canonicalPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: parent_path must be an accessible directory", ErrInvalidLocalRepositoryInitialization)
	}
	if info.Mode().Perm()&0o222 == 0 {
		return "", fmt.Errorf("%w: parent directory is not writable", ErrInvalidLocalRepositoryInitialization)
	}
	if err := validateLocalRepositoryParentChain(canonicalPath); err != nil {
		return "", err
	}
	return canonicalPath, nil
}

func validateLocalRepositoryParentChain(parentPath string) error {
	for path := parentPath; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
			!localRepositoryDirectoryOwnerTrusted(info) {
			return fmt.Errorf("%w: parent directory ownership is not trusted", ErrInvalidLocalRepositoryInitialization)
		}
		if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("%w: parent directory must not be shared writable", ErrInvalidLocalRepositoryInitialization)
		}
		if filepath.Dir(path) == path {
			return nil
		}
	}
}

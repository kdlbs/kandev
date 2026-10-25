package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrProjectRepositorySourceReaderUnavailable = errors.New("project repository source reader is unavailable")

// ProjectRepositorySources is the task service's Office-neutral project read
// result. Sources retain the project's configured order.
type ProjectRepositorySources struct {
	WorkspaceID string
	Sources     []string
}

// ProjectRepositorySourceReader reads the source list for an exact project ID.
type ProjectRepositorySourceReader interface {
	ReadProjectRepositorySources(context.Context, string) (ProjectRepositorySources, error)
}

func (s *Service) prepareProjectRepositorySources(ctx context.Context, req *CreateTaskRequest) error {
	if req.ParentID != "" || req.ProjectID == "" || req.Repositories != nil || req.WorkspacePath != "" {
		return nil
	}
	if req.WorkspacePolicy != nil && req.WorkspacePolicy.Mode == workspaceModeSharedGroup {
		return nil
	}
	if s.projectRepositorySourceReader == nil {
		return ErrProjectRepositorySourceReaderUnavailable
	}
	project, err := s.projectRepositorySourceReader.ReadProjectRepositorySources(ctx, req.ProjectID)
	if err != nil {
		return fmt.Errorf("read repository sources for project %q: %w", req.ProjectID, err)
	}
	if project.WorkspaceID != req.WorkspaceID {
		return fmt.Errorf("project %q does not belong to workspace %q", req.ProjectID, req.WorkspaceID)
	}
	repositories, err := projectRepositoryInputs(project.Sources)
	if err != nil {
		return fmt.Errorf("invalid repository sources for project %q: %w", req.ProjectID, err)
	}
	req.Repositories = repositories
	req.projectRepositoryDefaults = true
	return nil
}

func projectRepositoryInputs(sources []string) ([]TaskRepositoryInput, error) {
	inputs := make([]TaskRepositoryInput, 0, len(sources))
	for index, rawSource := range sources {
		source := strings.TrimSpace(rawSource)
		if source == "" {
			return nil, fmt.Errorf("source %d is empty", index+1)
		}
		if isRemoteProjectRepositorySource(source) {
			inputs = append(inputs, TaskRepositoryInput{RemoteURL: source})
			continue
		}
		canonicalPath, _, err := resolveExplicitLocalRepositoryPath(source)
		if err != nil {
			return nil, fmt.Errorf("source %d: %w", index+1, err)
		}
		inputs = append(inputs, TaskRepositoryInput{LocalPath: canonicalPath})
	}
	return inputs, nil
}

func isRemoteProjectRepositorySource(source string) bool {
	if isWindowsDrivePath(source) {
		return false
	}
	if strings.Contains(source, "://") || strings.HasPrefix(strings.ToLower(source), "git@") {
		return true
	}
	if _, _, _, _, err := parseRemoteRepositoryURL(source, ""); err == nil {
		return true
	}
	separator := strings.IndexByte(source, ':')
	at := strings.IndexByte(source, '@')
	pathSeparator := strings.IndexAny(source, "/\\")
	return at > 0 && separator > at && (pathSeparator < 0 || separator < pathSeparator)
}

func isWindowsDrivePath(source string) bool {
	return len(source) >= 2 &&
		((source[0] >= 'a' && source[0] <= 'z') || (source[0] >= 'A' && source[0] <= 'Z')) &&
		source[1] == ':'
}

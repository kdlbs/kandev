package service

import (
	"fmt"
	"strings"
)

const (
	WorkspaceLayoutRepository = "repository"
	WorkspaceLayoutTaskRoot   = "task_root"
)

var ErrInvalidInitialWorkspaceLayout = fmt.Errorf("invalid initial workspace layout")

func NormalizeInitialWorkspaceLayout(requested string, repositoryCount int) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != WorkspaceLayoutRepository && requested != WorkspaceLayoutTaskRoot {
		return "", fmt.Errorf("%w: %q", ErrInvalidInitialWorkspaceLayout, requested)
	}
	if repositoryCount == 0 {
		if requested == WorkspaceLayoutTaskRoot {
			return "", fmt.Errorf("%w: task_root requires at least one repository", ErrInvalidInitialWorkspaceLayout)
		}
		return "", nil
	}
	if repositoryCount > 1 {
		return WorkspaceLayoutTaskRoot, nil
	}
	if requested == "" {
		return WorkspaceLayoutRepository, nil
	}
	return requested, nil
}

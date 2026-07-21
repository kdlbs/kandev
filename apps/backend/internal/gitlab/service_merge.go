package gitlab

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// MergeMR accepts a merge request, validating against the project's allowed
// merge methods. `method` is one of "merge", "rebase_merge", "ff", or "squash"
// (squash performs a squash regardless of project method).
func (s *Service) MergeMR(ctx context.Context, projectPath string, iid int, method, squashCommitMessage string) (*MR, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	methods, err := client.GetProjectMergeMethods(ctx, projectPath)
	if err != nil {
		return nil, fmt.Errorf("get project merge methods: %w", err)
	}
	squash := false
	switch method {
	case "merge":
		if !methods.Merge {
			return nil, fmt.Errorf("project does not allow merge commits")
		}
	case "rebase_merge":
		if !methods.RebaseMerge {
			return nil, fmt.Errorf("project does not allow rebase merge")
		}
	case "ff":
		if !methods.FastForward {
			return nil, fmt.Errorf("project does not allow fast-forward merge")
		}
	case "squash":
		if !methods.AllowSquash {
			return nil, fmt.Errorf("project does not allow squash merge")
		}
		squash = true
	case "":
		// caller didn't pick — use squash if available, else default.
		squash = methods.AllowSquash
	default:
		return nil, fmt.Errorf("unknown merge method: %q", method)
	}
	mr, err := client.MergeMR(ctx, projectPath, iid, squash, squashCommitMessage)
	if err != nil {
		return nil, err
	}
	s.logger.Info("merged GitLab MR", zap.String("project", projectPath), zap.Int("iid", iid))
	return mr, nil
}

// GetProjectMergeMethods proxies to the client and reports allowed merge methods.
func (s *Service) GetProjectMergeMethods(ctx context.Context, projectPath string) (*ProjectMergeMethods, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.GetProjectMergeMethods(ctx, projectPath)
}

// SubmitMRApproval approves an MR.
func (s *Service) SubmitMRApproval(ctx context.Context, projectPath string, iid int) error {
	client := s.Client()
	if client == nil {
		return ErrNoClient
	}
	if err := client.SubmitMRApproval(ctx, projectPath, iid); err != nil {
		return fmt.Errorf("approve MR: %w", err)
	}
	return nil
}

// SubmitMRUnapproval revokes the authenticated user's approval of an MR.
func (s *Service) SubmitMRUnapproval(ctx context.Context, projectPath string, iid int) error {
	client := s.Client()
	if client == nil {
		return ErrNoClient
	}
	if err := client.SubmitMRUnapproval(ctx, projectPath, iid); err != nil {
		return fmt.Errorf("unapprove MR: %w", err)
	}
	return nil
}

// SetMRLabels replaces the labels on an MR.
func (s *Service) SetMRLabels(ctx context.Context, projectPath string, iid int, labels []string) error {
	client := s.Client()
	if client == nil {
		return ErrNoClient
	}
	return client.SetMRLabels(ctx, projectPath, iid, labels)
}

// SetMRAssignees replaces the assignees on an MR.
func (s *Service) SetMRAssignees(ctx context.Context, projectPath string, iid int, assigneeIDs []int) error {
	client := s.Client()
	if client == nil {
		return ErrNoClient
	}
	return client.SetMRAssignees(ctx, projectPath, iid, assigneeIDs)
}

// GetMRFiles lists files changed in an MR.
func (s *Service) GetMRFiles(ctx context.Context, projectPath string, iid int) ([]MRFile, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.ListMRFiles(ctx, projectPath, iid)
}

// GetMRCommits lists commits in an MR.
func (s *Service) GetMRCommits(ctx context.Context, projectPath string, iid int) ([]MRCommitInfo, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.ListMRCommits(ctx, projectPath, iid)
}

// ListUserProjects lists projects the user is a member of.
func (s *Service) ListUserProjects(ctx context.Context) ([]Project, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.ListUserProjects(ctx)
}

// SearchProjects searches all visible projects by name.
func (s *Service) SearchProjects(ctx context.Context, query string, limit int) ([]Project, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.SearchProjects(ctx, query, limit)
}

// ListProjectBranches lists branches for a project.
func (s *Service) ListProjectBranches(ctx context.Context, projectPath string) ([]RepoBranch, error) {
	client := s.Client()
	if client == nil {
		return nil, ErrNoClient
	}
	return client.ListProjectBranches(ctx, projectPath)
}

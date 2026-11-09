package gitlab

import (
	"context"
	"errors"
	"fmt"
)

// MRLiveRead is an uncached read of a merge request.
type MRLiveRead struct {
	Status                *MRStatus
	UnresolvedDiscussions int
}

// ReadMRLiveForAutomation reads the merge request from the provider, scoped to
// the workspace's credentials and the expected host. Every part is read
// completely or the call fails; nothing is served from a cache.
func (s *Service) ReadMRLiveForAutomation(ctx context.Context, workspaceID, host, projectPath string, mrIID int) (*MRLiveRead, error) {
	var read *MRLiveRead
	err := s.RunWithWorkspaceClient(ctx, workspaceID, host, func(client Client) error {
		status, err := client.GetMRStatus(ctx, projectPath, mrIID)
		if err != nil {
			return err
		}
		if status == nil || status.MR == nil {
			return fmt.Errorf("merge request %s!%d: provider returned no merge request", projectPath, mrIID)
		}
		if err := requireHeadPipeline(ctx, client, projectPath, status); err != nil {
			return err
		}
		discussions, err := client.ListMRDiscussions(ctx, projectPath, mrIID, nil)
		if err != nil {
			return err
		}
		read = &MRLiveRead{Status: status, UnresolvedDiscussions: countUnresolvedDiscussions(discussions)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return read, nil
}

// ProjectMergePolicy is a project's merge settings: the allowed methods and
// the raw squash option (never, default_off, default_on, always).
type ProjectMergePolicy struct {
	Methods      ProjectMergeMethods
	SquashOption string
}

// ProjectMergePolicyForAutomation reads the project's merge settings. A client
// that cannot report the squash option leaves it empty, which reads as
// default_off.
func (s *Service) ProjectMergePolicyForAutomation(ctx context.Context, workspaceID, host, projectPath string) (ProjectMergePolicy, error) {
	var policy ProjectMergePolicy
	err := s.RunWithWorkspaceClient(ctx, workspaceID, host, func(client Client) error {
		methods, err := client.GetProjectMergeMethods(ctx, projectPath)
		if err != nil {
			return err
		}
		policy.Methods = *methods
		if reader, ok := client.(squashOptionReader); ok {
			policy.SquashOption, err = reader.GetProjectSquashOption(ctx, projectPath)
		}
		return err
	})
	return policy, err
}

// ReadProjectProtectionForAutomation reads what the project enforces for
// merging into a branch; an empty branch is the default branch.
func (s *Service) ReadProjectProtectionForAutomation(ctx context.Context, workspaceID, host, projectPath, branch string) (*ProjectProtection, error) {
	var out *ProjectProtection
	err := s.RunWithWorkspaceClient(ctx, workspaceID, host, func(client Client) error {
		reader, ok := client.(projectProtectionReader)
		if !ok {
			return errors.New("gitlab client cannot read project settings")
		}
		var err error
		out, err = reader.GetProjectProtection(ctx, projectPath, branch)
		return err
	})
	return out, err
}

// ListBranchCommitsForAutomation lists one page of 100 commits of a branch,
// newest first. more reports a full page.
func (s *Service) ListBranchCommitsForAutomation(ctx context.Context, workspaceID, host, projectPath, branch string, page int) (commits []MRCommitInfo, more bool, err error) {
	err = s.RunWithWorkspaceClient(ctx, workspaceID, host, func(client Client) error {
		lister, ok := client.(branchCommitLister)
		if !ok {
			return errors.New("gitlab client cannot list branch commits")
		}
		var listErr error
		commits, listErr = lister.ListBranchCommits(ctx, projectPath, branch, page)
		return listErr
	})
	return commits, len(commits) >= 100, err
}

// requireHeadPipeline downgrades a pipeline state that does not belong to the
// merge request head: the newest pipeline of the branch must carry the head
// SHA, otherwise the state reads as pending.
func requireHeadPipeline(ctx context.Context, client Client, projectPath string, status *MRStatus) error {
	if status.PipelineState == "" || status.MR.HeadSHA == "" {
		return nil
	}
	pipelines, err := client.ListPipelines(ctx, projectPath, status.MR.HeadBranch)
	if err != nil {
		return err
	}
	if len(pipelines) == 0 || pipelines[0].SHA != status.MR.HeadSHA {
		status.PipelineState = "pending"
	}
	return nil
}

package github

import "context"

type MergeAccessState string

const (
	MergeAccessAllowed MergeAccessState = "allowed"
	MergeAccessDenied  MergeAccessState = "denied"
	MergeAccessUnknown MergeAccessState = "unknown"
)

// RepoMergeAccess describes repository access for the manual mutation actor.
// GitHub still enforces token restrictions and branch rules at mutation time.
type RepoMergeAccess struct {
	State MergeAccessState `json:"state"`
	Actor *AuthPrincipal   `json:"actor"`
}

func (s *Service) GetRepoMergeAccessForWorkspace(
	ctx context.Context, workspaceID, userID, owner, repo string,
) (RepoMergeAccess, error) {
	access := RepoMergeAccess{State: MergeAccessUnknown}
	resolved, err := s.resolvePersonalWriteClient(ctx, workspaceID, userID, owner, repo)
	if err != nil {
		return access, err
	}
	access.Actor = &resolved.Principal
	if resolved.Principal.Kind == AuthPrincipalApp &&
		(!resolved.Capabilities[CapabilityGitWrite] || !resolved.Capabilities[CapabilityPullRequestWrite]) {
		access.State = MergeAccessDenied
		return access, nil
	}
	reader, ok := resolved.Client.(repositoryDetailsReader)
	if !ok {
		return access, nil
	}
	repository, err := reader.GetRepository(ctx, owner, repo)
	if err != nil || repository == nil {
		return access, err
	}
	if resolved.Principal.Kind == AuthPrincipalApp {
		access.State = MergeAccessAllowed
	} else if resolved.Principal.Kind == AuthPrincipalHuman && repository.PushAccessKnown {
		access.State = MergeAccessDenied
		if repository.PushAccess {
			access.State = MergeAccessAllowed
		}
	}
	return access, nil
}

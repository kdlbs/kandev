package github

import "strings"

// githubRepositoryResponse is the shared subset returned by GET and POST
// repository endpoints. Keep the provider response private so only the
// credential-free projection crosses service boundaries.
type githubRepositoryResponse struct {
	ID            int64  `json:"id"`
	NodeID        string `json:"node_id"`
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	CloneURL      string `json:"clone_url"`
	HTMLURL       string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
	Fork          bool   `json:"fork"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
	Parent *struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"parent"`
	Permissions struct {
		Push  bool `json:"push"`
		Admin bool `json:"admin"`
	} `json:"permissions"`
}

func projectGitHubRepository(raw githubRepositoryResponse) *GitHubRepository {
	repository := &GitHubRepository{
		ID:            raw.ID,
		NodeID:        raw.NodeID,
		FullName:      raw.FullName,
		Owner:         raw.Owner.Login,
		Name:          raw.Name,
		CloneURL:      raw.CloneURL,
		HTMLURL:       raw.HTMLURL,
		DefaultBranch: raw.DefaultBranch,
		Fork:          raw.Fork,
		PushAccess:    raw.Permissions.Push,
		AdminAccess:   raw.Permissions.Admin,
	}
	if raw.Parent != nil {
		repository.ParentID = raw.Parent.ID
		repository.ParentFullName = raw.Parent.FullName
	}
	return repository
}

func copyGitHubRepository(repository *GitHubRepository) *GitHubRepository {
	if repository == nil {
		return nil
	}
	copy := *repository
	return &copy
}

func repositoryKeyFromFullName(fullName string) repoKey {
	parts := strings.SplitN(strings.TrimSpace(fullName), "/", 2)
	if len(parts) != 2 {
		return repoKey{Owner: fullName}
	}
	return repoKey{Owner: parts[0], Repo: parts[1]}
}

package backendapp

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// noteTasks and noteRepositories are the board reads a citation check needs.
type noteTasks interface {
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
}

type noteTurns interface {
	TurnExists(ctx context.Context, coordinatorID, turnID string) (bool, error)
}

type notePullRequests interface {
	ReadLive(ctx context.Context, ws string, key mergestate.ChangeKey) (mergestate.Snapshot, error)
}

// coordinatorNoteResolver implements coordinator.NoteResolver over the board,
// the repositories' local clones and the provider lookups. It never fetches.
type coordinatorNoteResolver struct {
	tasks noteTasks
	repos mergeRepositories
	turns noteTurns
	prs   notePullRequests
}

func (r *coordinatorNoteResolver) Open(_ context.Context, workspaceID, coordinatorID string) coordinator.CitationChecker {
	return &noteChecker{r: r, ws: workspaceID, coordinatorID: coordinatorID, heads: map[string]headResult{}}
}

type headResult struct {
	sha   string
	state coordinator.CitationState
}

type noteChecker struct {
	r             *coordinatorNoteResolver
	ws            string
	coordinatorID string
	mu            sync.Mutex
	heads         map[string]headResult
}

func (c *noteChecker) Check(ctx context.Context, cite coordinator.NoteCitation) coordinator.CitationState {
	switch cite.Kind {
	case coordinator.CitationTurn:
		ok, err := c.r.turns.TurnExists(ctx, c.coordinatorID, cite.ID)
		return presence(ok, err)
	case coordinator.CitationTask:
		return c.checkTask(ctx, cite.ID)
	case coordinator.CitationPR:
		return c.checkPR(ctx, cite)
	case coordinator.CitationFile:
		return c.checkFile(ctx, cite)
	}
	return coordinator.CitationUnavailable
}

func presence(ok bool, err error) coordinator.CitationState {
	switch {
	case err != nil:
		return coordinator.CitationUnavailable
	case ok:
		return coordinator.CitationPresent
	}
	return coordinator.CitationAbsent
}

func (c *noteChecker) checkTask(ctx context.Context, id string) coordinator.CitationState {
	task, err := c.r.tasks.GetTask(ctx, id)
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return coordinator.CitationAbsent
	}
	if err != nil {
		return coordinator.CitationUnavailable
	}
	return presence(task != nil && task.WorkspaceID == c.ws, nil)
}

// repository returns the workspace repository, or the state to report.
func (c *noteChecker) repository(ctx context.Context, id string) (*taskmodels.Repository, coordinator.CitationState, bool) {
	repo, err := c.r.repos.GetRepository(ctx, id)
	if errors.Is(err, repoerrors.ErrRepositoryNotFound) || (err == nil && (repo == nil || repo.WorkspaceID != c.ws)) {
		return nil, coordinator.CitationAbsent, false
	}
	if err != nil {
		return nil, coordinator.CitationUnavailable, false
	}
	return repo, coordinator.CitationPresent, true
}

func (c *noteChecker) checkPR(ctx context.Context, cite coordinator.NoteCitation) coordinator.CitationState {
	repo, st, ok := c.repository(ctx, cite.RepositoryID)
	if !ok {
		return st
	}
	if c.r.prs == nil || (repo.Provider != mergeProviderGitHub && repo.Provider != mergeProviderGitLab) {
		return coordinator.CitationUnavailable
	}
	_, err := c.r.prs.ReadLive(ctx, c.ws, mergestate.ChangeKey{Provider: repo.Provider, RepoID: repo.ID, Number: cite.Number})
	var apiErr *github.GitHubAPIError
	switch {
	case err == nil:
		return coordinator.CitationPresent
	case repo.Provider == mergeProviderGitHub && errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
		return coordinator.CitationAbsent
	}
	return coordinator.CitationUnavailable
}

// gitProbe runs `git rev-parse --verify --quiet rev` in dir: the object id,
// or absent when git reports the revision missing (exit 1), or unavailable.
func gitProbe(ctx context.Context, dir, rev string) (string, coordinator.CitationState) {
	cmd := subproc.NewGitCommand(ctx, "rev-parse", "--verify", "--quiet", rev)
	cmd.Dir = dir
	out, err := subproc.RunGitOutputClass(ctx, subproc.GitBackground, cmd)
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return strings.TrimSpace(string(out)), coordinator.CitationPresent
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
		return "", coordinator.CitationAbsent
	}
	return "", coordinator.CitationUnavailable
}

// head captures the tip of the default branch once per repository: the local
// branch ref, else the remote-tracking ref. An empty path, a missing clone or
// an unreadable ref is unavailable, never absence.
func (c *noteChecker) head(ctx context.Context, repo *taskmodels.Repository) headResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h, ok := c.heads[repo.ID]; ok {
		return h
	}
	h := headResult{state: coordinator.CitationUnavailable}
	if repo.LocalPath != "" && repo.DefaultBranch != "" {
		for _, ref := range []string{"refs/heads/" + repo.DefaultBranch, "refs/remotes/origin/" + repo.DefaultBranch} {
			sha, st := gitProbe(ctx, repo.LocalPath, ref+"^{commit}")
			if st == coordinator.CitationPresent {
				h = headResult{sha: sha, state: st}
				break
			}
		}
	}
	c.heads[repo.ID] = h
	return h
}

// checkFile is valid only when the cited commit exists, the file exists in it
// and its blob at HEAD equals the cited commit's blob.
func (c *noteChecker) checkFile(ctx context.Context, cite coordinator.NoteCitation) coordinator.CitationState {
	repo, st, ok := c.repository(ctx, cite.RepositoryID)
	if !ok {
		return st
	}
	head := c.head(ctx, repo)
	if head.state != coordinator.CitationPresent {
		return coordinator.CitationUnavailable
	}
	if _, st := gitProbe(ctx, repo.LocalPath, cite.SHA+"^{commit}"); st != coordinator.CitationPresent {
		return st
	}
	cited, st := gitProbe(ctx, repo.LocalPath, cite.SHA+":"+cite.Path)
	if st != coordinator.CitationPresent {
		return st
	}
	current, st := gitProbe(ctx, repo.LocalPath, head.sha+":"+cite.Path)
	if st != coordinator.CitationPresent {
		return st
	}
	return presence(cited == current, nil)
}

// wireCoordinatorNotes connects the citation resolver of agent-tier notes. It
// does nothing unless notes are effective.
func wireCoordinatorNotes(svc *coordinator.Service, tasks noteTasks, repos mergeRepositories, gh *github.Service, gl *gitlab.Service) {
	if svc == nil || tasks == nil || repos == nil || !svc.NotesEnabled() {
		return
	}
	var ghImpl, glImpl providerMerge
	if gh != nil {
		ghImpl = githubMerge{api: gh}
	}
	if gl != nil {
		glImpl = gitlabMerge{api: gl}
	}
	svc.SetNoteResolver(&coordinatorNoteResolver{
		tasks: tasks, repos: repos, turns: svc, prs: newCoordinatorMergeProviders(repos, ghImpl, glImpl),
	})
}

## Pull request and merge request auto-linking

Kandev associates a task with a GitHub pull request or GitLab merge request on its session branch.
The association stores the task link and a watch for status updates.

Discovery starts from:
- Kandev's `worktree.create_pr` action, through `createdChangeAssociationRouter.associate`.
- A detected branch push, through `trackPushAndAssociatePR` and the provider-specific detection function. Attempts use successive delays of 0, 30, and 60 seconds.
- The on-demand WebSocket actions `github.check_session_pr` and `gitlab.check_session_mr`.

Manual URL linking is an explicit action, not discovery: use `POST /api/v1/github/task-prs`
or `POST /api/v1/gitlab/task-mrs`.

Pollers refresh existing watches. They may resolve a placeholder watch with PR number or MR IID 0
when the change request appears later. They do not create a watch without an earlier discovery path.
A web-created change request with no observed push or placeholder cannot be discovered by the poller.

Dispatch by the persisted repository provider. A GitHub push must not call GitLab and vice versa.
Scope associations and watches by `(repository_id, branch)` so sibling repositories and branches remain independent.

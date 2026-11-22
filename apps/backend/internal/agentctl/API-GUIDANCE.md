## API Groups

agentctl exposes these route groups (see `server/api/`):
- `/health`, `/info`, `/status`, `/runtime/storage` - Health, status, and the read-only executor filesystem advisory
- `/instances/*` - Multi-instance management
- `/processes/*` - Agent subprocess management (start/stop)
- `/agent/configure`, `/agent/stream` - Agent configuration and event streaming
- `/lsp/stream` - Task-host language-server stdio/WebSocket bridge
- `/git/*` - Git operations (status, commit, push, pull, rebase, stage, create PR, etc.)
- `/shell/*` - Shell session management
- `/workspace/*` - File operations, search, tree
- `/vscode/*` - VS Code integration proxy

The **control server** (`server/api/control_server.go`) is a separate router from
the per-instance ones above, and it owns the agent-survival surface:
- `/identity` - opaque per-launch server identity, advertised capability set,
  and this server's own resolved unowned period. Unauthenticated, because it is
  what decides whether to authenticate. It therefore carries no filesystem path.
- `/ownership/prove` - challenge/response proving this process holds the
  credential the caller's record names. Also unauthenticated, for the same
  reason, and safe because a proof is a keyed digest over a caller-chosen
  challenge and cannot be inverted to the credential. Derivation is shared with
  the backend through `internal/common/ownershipproof`.
- `/api/v1/ownership/claim`, `/rotate`, `/confirm`, `/shutdown`, `/details` -
  the authenticated ownership operations (`claim` is the renewal an owning
  backend repeats on a cadence derived from the unowned period), plus the
  filesystem paths kept off `/identity`.

If no control credential is configured, `controlCredentialAuth` allows all requests.
When a credential is configured, `/health`, `/auth/handshake`, `/identity`, and `/ownership/prove`
are exempt entirely. The paths in `adoptionOnlyPaths` (`rotate`, `shutdown`)
additionally accept the **superseded** credential until the rotation that
replaced it is confirmed, so a backend that crashed mid-rotation can still
adopt. Every other authenticated route accepts only the current credential.

## Multi-repository Git and review payloads

Preserve the repository-scoped fields `repository_name`, `base_ref`, and
`is_submodule` in aggregate results; nested submodule comparisons stay anchored
to the parent gitlink. The focused contract tests are in
`server/api/git_multi_repo_review_test.go`.

## Pull request creation (`server/process/git_pr_providers.go`)

`GitOperator.CreatePR` picks a host CLI from `origin`:

| Remote host | CLI | Notes |
|-------------|-----|-------|
| `github.com`, `*.github.com` | `gh pr create` | Requires `gh` on `PATH` (included in the Kandev Docker image). |
| `gitlab.com`, configured self-managed host | `glab mr create` or REST v4 | `KANDEV_GITLAB_HOST` binds detection/auth to the workspace origin. `GITLAB_TOKEN` is used only for that exact host; the REST fallback resolves the project default branch and reuses an existing open source/target MR on retry. |
| Other hosts | — | Returns an unsupported-remote error. Plugin-owned providers, such as Azure Repos through `kandev-plugin-azure-devops`, create pull requests through the plugin's `createChangeRequest` callback or agent tools, not agentctl. |

GitHub PR and GitLab MR URLs trigger backend association callbacks. The web UI keeps a session-scoped pending URL so the changes panel hides the create action after any successful provider response.

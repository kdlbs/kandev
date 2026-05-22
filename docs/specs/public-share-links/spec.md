---
status: draft
created: 2026-05-21
owner: Kandev
---

# Public Share Links (v0)

## Why

When a user finishes a noteworthy task — an interesting bug-fix transcript, a
clean refactor walkthrough, a teaching example — they have no way to send it
to a teammate or post it externally without screenshotting message by message.
A one-click "Share" that produces a stable public URL turns every completed
task into a shareable artifact, drives word-of-mouth growth, and gives users
a portable record of work they cared about.

v0 ships with zero new server infrastructure: the snapshot is uploaded to a
**secret-by-default GitHub Gist** on the user's own account, and the gist URL
*is* the share link. A future v1 will introduce a hosted `share.kandev.com`
service; the snapshot format produced in v0 is forward-compatible so the same
blob can be re-posted to the hosted service without code changes.

## What

- A "Share" button appears in the chat panel header for any session past
  the pre-history states (`CREATED` / `STARTING`). The button is hidden
  while the session is still warming up — there is nothing worth
  publishing yet — and visible for every other state (`RUNNING`, `IDLE`,
  `WAITING_FOR_INPUT`, `COMPLETED`, `FAILED`, `CANCELLED`). Users can
  share an in-progress conversation if they want to; the backend mirrors
  this rule (see Failure modes).
- Clicking Share opens a dialog with a **mandatory preview-and-confirm step**:
  the dialog renders the redacted snapshot in a read-only viewer that reuses
  the existing session message components, plus a visible warning that anyone
  with the link will be able to view the conversation.
- The user only publishes by clicking an explicit confirm button in the
  dialog ("Publish to GitHub Gist"). The dialog never auto-publishes on open.
- On publish, kandev builds a **frozen snapshot** of the session and uploads
  it as a secret GitHub Gist on the user's authenticated GitHub account. The
  dialog then shows the gist URL with a "Copy" button.
- A snapshot is self-contained: it survives the underlying task, session, or
  workspace being deleted from kandev. It MUST NOT contain foreign keys to
  live kandev rows.
- Every snapshot passes through a redaction pass before being uploaded:
  - Absolute paths under the session's worktree root are rewritten to
    repo-relative paths (`/Users/foo/proj/src/x.ts` → `src/x.ts`).
  - Known secret shapes are stripped:
    `sk-[a-zA-Z0-9]{20,}`, `ghp_[A-Za-z0-9]{36,}`, `gho_[A-Za-z0-9]{36,}`,
    `github_pat_[A-Za-z0-9_]{36,}`, `AKIA[0-9A-Z]{16}`.
  - The contents of any `.env*` file read or written via tool calls are
    replaced with the placeholder `"[redacted: .env contents]"`.
  - The `env` field on shell tool-call payloads is dropped entirely.
- A list of "Active shares for this session" is reachable from the task
  details surface, with a per-row **Revoke** action that deletes the gist
  and marks the share row revoked.
- Revoke is best-effort: if the gist is already gone on GitHub, the local
  row is still marked revoked.
- The Share button and the publish flow require an authenticated GitHub
  user account on kandev (the same OAuth/PAT credential that powers the
  existing GitHub integration). If GitHub credentials are missing, the
  Share button surfaces a blocking error referencing the GitHub settings
  page rather than opening the dialog.

## Data model

A new SQLite table `task_shares` records every share that has been
published. The frozen snapshot itself lives on the gist, not in the
kandev DB; the table only records the metadata needed to manage and
revoke shares.

```text
task_shares
  id                  TEXT       PRIMARY KEY (uuid)
  task_session_id     TEXT       NOT NULL  -- soft reference; no FK constraint
  backend             TEXT       NOT NULL  -- "github_gist" in v0
  external_id         TEXT       NOT NULL  -- gist ID
  external_url        TEXT       NOT NULL  -- public gist URL (the share link)
  snapshot_size_bytes INTEGER    NOT NULL
  created_at          TIMESTAMP  NOT NULL
  revoked_at          TIMESTAMP  NULL
  view_count          INTEGER    NOT NULL DEFAULT 0
```

Notes:
- `task_session_id` is a soft reference. Deleting the task does not cascade.
  Lookups by `task_session_id` filter out rows whose session no longer
  exists.
- `backend` is reserved for forward-compat: v1 will add `"hosted"`. The
  controller MUST reject unknown backends.
- `view_count` is present on the schema for v1 but is always `0` in v0 (the
  gist host tracks its own views; we do not poll for them).
- The migration is applied through the standard `db.MigrateLogger.Apply`
  pattern documented in CLAUDE.md.

### Snapshot document

The snapshot uploaded to the gist is `snapshot.json` (machine-readable
contract), `share.html` (the kandev-styled rendering shown when the share
URL is opened), and `README.md` (fallback for users who land on the gist
directly). The JSON schema is the durable contract; the HTML and README
are derived from it.

```text
Snapshot
  version          int       -- 1
  kandev_version   string    -- version that produced the snapshot
  exported_at      timestamp
  task
    title          string
    workflow_step  string|null
  session
    agent_type     string    -- e.g. "claude-acp"
    model          string|null
    executor_type  string    -- "local_pc" | "local_docker" | "sprites" | ...
    started_at     timestamp
    completed_at   timestamp
  messages: array of Message
    Message
      role         "user" | "assistant" | "system"
      ts           timestamp
      blocks: array of Block
        Block      one of: TextBlock | ToolCallBlock | ToolResultBlock | DiffBlock
  redaction
    applied_rules  array<string>  -- e.g. ["abs-path", "secret-sk", "env-vars"]
```

Block shapes are intentionally narrow: a TextBlock carries `text`; a
ToolCallBlock carries `name`, `args` (already redacted); a ToolResultBlock
carries `output` (already redacted) and a boolean `truncated`; a DiffBlock
carries `path` (repo-relative) and `unified_diff`.

The snapshot has no references back into kandev's database, no agent IDs,
no user IDs. It is intentionally portable.

## API surface

Endpoints live under the `/api/v1` prefix to match the rest of the kandev
HTTP surface. kandev's HTTP layer has no per-request user middleware
today — authorisation is "this kandev instance can reach this task in its
local store", same as every other task endpoint.

```http
POST   /api/v1/tasks/:taskId/sessions/:sessionId/shares
       -> 201 { id, url, created_at, snapshot_size_bytes, revoked_at? }
       Builds and uploads a snapshot. Idempotency is not provided in v0;
       calling twice creates two gists.

POST   /api/v1/tasks/:taskId/sessions/:sessionId/shares?dry_run=true
       -> 200 <Snapshot JSON>
       Returns the redacted snapshot that would be uploaded, without
       calling the gist backend or persisting a row. Used by the modal
       preview so the user sees exactly what will be published.

GET    /api/v1/tasks/:taskId/sessions/:sessionId/shares
       -> 200 { shares: [{ id, url, created_at, revoked_at, snapshot_size_bytes }] }
       Lists every share row (including revoked) for the session. The
       `url` field is always normalised to the canonical githack rendering
       URL (gist.githack.com/<owner>/<id>/raw/share.html) regardless of
       what's stored; legacy gistpreview.github.io rows are passed
       through (re-pinned to /share.html when the filename is missing).

DELETE /api/v1/shares/:shareId
       -> 204
       Revokes a share: deletes the gist and marks the row revoked.
```

Internal Go surface:

```go
// internal/task/share
type Snapshot struct{ /* shape above */ }
type Service struct{ /* unexported */ }

func BuildSnapshot(ctx context.Context, repo TaskReader, taskSessionID, kandevVersion string) (*Snapshot, error)

func (s *Service) CreateShare(ctx context.Context, taskSessionID string) (*Share, error)
func (s *Service) RevokeShare(ctx context.Context, shareID string) error
func (s *Service) ListBySession(ctx context.Context, taskSessionID string) ([]*Share, error)
func (s *Service) PreviewSnapshot(ctx context.Context, taskSessionID string) (*Snapshot, error)

type Backend interface {
    Name() string
    Upload(ctx context.Context, snap *Snapshot) (externalID, externalURL string, err error)
    Delete(ctx context.Context, externalID string) error
}
```

### Share URL format

The user-facing share URL routes through **gist.githack.com** — a
Cloudflare-backed CDN that proxies the gist's raw file content directly.

We initially shipped via `gistpreview.github.io` (a static GitHub-Pages
renderer that fetches gists through the public GitHub API), but the gists
API returns `content: ""` for any file past GitHub's per-response content
budget (~1 MB combined across files). For non-trivial sessions, the
generated `share.html` lands past that budget, and gistpreview renders a
blank page. githack proxies the raw HTML directly, so even multi-megabyte
share.html renders correctly.

The trade-off is that githack shows a one-time anti-phishing interstitial
the first time a user visits any githack URL on a given device — accepted
as the price of fidelity for big tasks.

Format: `https://gist.githack.com/<owner>/<id>/raw/share.html`

The `/raw/share.html` suffix is required: githack can't pick a default
file without an explicit path.

`displayURL` on the API layer normalises stored URL formats to the
canonical githack-with-share.html form on every response:

- `gist.github.com/<owner>/<id>` → githack
- `gist.githack.com/<owner>/<id>` (no filename) → re-pinned to `/raw/share.html`
- Legacy `gistpreview.github.io/?<id>/share.html` → passed through
  unchanged. The owner is not recoverable from a gistpreview URL, so we
  cannot auto-upgrade. Users can revoke + re-share to get a githack URL
  for those rows.

## State machine

A share row has two states:

```text
created --[user clicks Revoke OR DELETE /api/shares/:id]--> revoked
```

- `created` (`revoked_at IS NULL`): the gist is live; anyone with the URL
  can view it.
- `revoked` (`revoked_at IS NOT NULL`): the row is preserved for audit, but
  the gist has been deleted and the URL returns 404.

There is no "draft" or "scheduled" state; publish is synchronous.

## Permissions

- kandev's HTTP layer has no per-request user identity today; the
  authorisation contract for share endpoints is the same as for every
  other task endpoint — "this kandev instance can see this task in its
  local store." If/when kandev grows multi-user, the same row filter that
  scopes tasks scopes shares.
- The GitHub credential used for the upload is the credential bound to
  the running kandev instance. Shares are owned by that GitHub account,
  not by a kandev service account.
- No "team share" or "delegate to another user" surface in v0.

## Failure modes

- **Session has no shareable content yet** (state is `CREATED` or
  `STARTING` — i.e., the agent hasn't run) when POSTing a share → API
  returns `409 Conflict` with `code: "session_not_shareable"`. The Share
  button is hidden in the UI for these states; the server enforces it as
  the source of truth. Every other state (`RUNNING`, `IDLE`,
  `WAITING_FOR_INPUT`, `COMPLETED`, `FAILED`, `CANCELLED`) is allowed —
  users can share an in-progress conversation if they want to.
- **No GitHub credential** on the kandev instance → API returns
  `412 Precondition Failed` with `code: "github_credential_missing"`.
  The UI surfaces a CTA to the GitHub settings page.
- **GitHub API failure during upload** (network, rate limit, 5xx) → API
  returns `502 Bad Gateway` with the upstream message. No row is
  written. The user can retry; no partial state is left behind.
- **Snapshot exceeds the gist size limit** (GitHub caps individual files
  at 100 MiB; we set an internal soft cap of 10 MiB on `snapshot.json` to
  stay well within practical limits) → API returns `413 Payload Too Large`
  with `code: "snapshot_too_large"`. No upload is attempted.
- **Revoke when the gist no longer exists on GitHub** (manually deleted by
  the user from gist.github.com) → the row is still marked revoked and
  the endpoint returns `204`. The discrepancy is logged at INFO level.
- **Revoke when the GitHub credential has been removed** → the local row
  is still marked revoked (so the share disappears from the UI list); a
  warning is logged that the upstream gist may still exist. The user is
  surfaced a non-blocking notice in the UI suggesting they delete the
  gist manually from github.com.
- **Redaction regex failure** (panic in a regex) → MUST NOT happen; the
  redaction code is covered by tests for each rule. As a defense in depth,
  any panic in the redaction pipeline aborts the publish with
  `500 Internal Server Error` and no upload occurs.

## Persistence guarantees

- `task_shares` rows survive kandev restarts (standard SQLite durability).
- The snapshot uploaded to the gist is **frozen at publish time** — later
  edits to the task or session do not change what is on the gist. The
  only way to update a published snapshot is to publish a new one.
- No background workers, queues, or schedulers are introduced by this
  feature. Publish and revoke are synchronous request handlers.
- If kandev crashes mid-publish (between gist upload and DB insert), the
  gist will exist on GitHub but no `task_shares` row will reference it.
  The user can recover by deleting the gist manually from github.com.
  v0 does not attempt to reconcile orphaned gists.

## Scenarios

- **GIVEN** a session past the pre-history states (anything other than
  `CREATED` / `STARTING`), **WHEN** the task chat panel renders, **THEN**
  a Share button is visible in the panel header.

- **GIVEN** a session in `CREATED` or `STARTING` state, **WHEN** the task
  chat panel renders, **THEN** the Share button is not visible.

- **GIVEN** the Share dialog opens for a shareable session, **THEN**
  the preview shows the redacted snapshot in head+tail form (first 8 +
  last 8 messages, with a "…N hidden in this preview…" indicator when
  the session is longer), the warning Alert is shown, and the publish
  button is enabled.

- **GIVEN** the preview dialog is open, **WHEN** the user clicks "Publish
  to GitHub Gist", **THEN** kandev uploads a secret gist containing
  `snapshot.json`, `share.html`, and `README.md`, inserts a `task_shares`
  row, and the dialog displays the gist.githack.com URL with a copy
  button.

- **GIVEN** an active share row, **WHEN** the user clicks Revoke, **THEN**
  the gist is deleted from GitHub, the row is updated with `revoked_at`,
  and the row disappears from the active-shares list.

- **GIVEN** an active share row whose gist was manually deleted on
  github.com, **WHEN** the user clicks Revoke, **THEN** the row is still
  marked revoked, the API returns 204, and an INFO log records the
  upstream 404.

- **GIVEN** a snapshot that contains an absolute path under the session's
  worktree root, **WHEN** the snapshot is built, **THEN** the path appears
  in the snapshot as repo-relative and the `redaction.applied_rules` list
  contains `"abs-path"`.

- **GIVEN** a tool call argument containing the substring
  `sk-abcdefghijklmnopqrstuv0123456789`, **WHEN** the snapshot is built,
  **THEN** that substring is replaced with `[redacted]` in the snapshot
  and `redaction.applied_rules` contains `"secret-sk"`.

- **GIVEN** a shell tool call payload with a populated `env` field,
  **WHEN** the snapshot is built, **THEN** the `env` field is omitted from
  the snapshot payload.

- **GIVEN** a message whose content is wrapped in `<kandev-system>` tags
  (system-injected prompt context), **WHEN** the snapshot is built,
  **THEN** the wrapped portion is stripped from the snapshot via the
  shared `sysprompt.StripSystemContent` helper.

- **GIVEN** an internal agent message of type `log`, `status`, `progress`,
  `error`, `thinking`, `agent_plan`, `todo`, `permission_request`,
  `clarification_request`, or `script_execution`, **WHEN** the snapshot
  is built, **THEN** the message is dropped — only user messages, plain
  agent prose (`message`/`content`/no type), and `tool_*` types make it
  into the share.

- **GIVEN** the user has not authenticated GitHub on their kandev account,
  **WHEN** they click Share, **THEN** the dialog does not open; an inline
  error references the GitHub settings page.

- **GIVEN** a `snapshot.json` larger than the 10 MiB soft cap, **WHEN**
  the user clicks Publish, **THEN** the request returns 413, no gist is
  created, and the dialog shows a "snapshot too large" message.

- **GIVEN** the GitHub gist API returns a 5xx during upload, **WHEN** the
  user clicks Publish, **THEN** the request returns 502, no
  `task_shares` row is written, and the dialog shows a retryable error.

## Out of scope

- The hosted `share.kandev.com` service. v0 is bring-your-own-store via
  gist. v1 introduces hosting.
- Editing or re-uploading a snapshot after publish. The only update path
  in v0 is "publish a new share".
- Accurate view count tracking. The schema reserves the column for v1 but
  v0 always reports `0`.
- Snapshot expiry policy / auto-revoke. Shares live until the user
  explicitly revokes them or deletes the gist on GitHub directly.
- Team / org / multi-user shares. v0 is per-user, owner-only.
- Reconciliation of orphaned gists (where a crash leaves a gist on GitHub
  with no kandev row).
- Embedding rich workspace context (file tree, repo HEAD SHA, agent
  config). v0 captures only what is needed to render the conversation.

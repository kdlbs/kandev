---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-002
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
  - REQ-INTEGRATIONS-PROMPT-SYNC-005
  - REQ-INTEGRATIONS-PROMPT-SYNC-006
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-008
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
  - REQ-INTEGRATIONS-PROMPT-SYNC-010
---

# Prompt Sync System Design

## Purpose and boundaries

Prompt Sync reconciles install-wide saved prompts from one repository
directory. The integration system owns the source configuration, the provider
reads, the failure policy, and the reconciliation rules. These adjacent
contracts are used but not owned here:

- The **prompt service** (`internal/prompts`) owns storage, name and content
  validation, `@name` matching, and the `prompts changed` event. This design
  adds ownership columns and sync-only write methods to it.
- **Provider connections** (`internal/github`, `internal/gitlab`) own
  credentials, host binding, and workspace-routed repository reads.
- **Workflow Sync** (`internal/workflowsync`) remains per workspace. It only
  gains the unresolved-reference warnings in
  REQ-INTEGRATIONS-PROMPT-SYNC-006, plus a shared helper package extracted
  from it without behavior changes.

### Decisions

- **One install-wide source.** Saved prompts are one global namespace
  (`custom_prompts.name` is unique), so the source matches that scope. This
  rules out cross-workspace name collisions by construction, and `@name`
  resolution stays workspace-agnostic.
- **A borrowed workspace credential.** Provider credentials exist only per
  workspace. The configuration names a credential workspace and reads go
  through that workspace's connection, so no new credential type is added.
- **A separate schedule.** Prompt Sync has its own **Sync now**, interval, and
  status. It never runs inside a Workflow Sync run, so the two failure
  domains stay independent.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-PROMPT-SYNC-001` | [Components](#components-and-responsibilities), [HTTP API](#http-api), [Security](#security) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-002` | [Prompt file parsing](#prompt-file-parsing) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-003` | [Reconciliation](#reconciliation) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-004` | [Prompt ownership](#prompt-ownership), [Frontend](#frontend) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-005` | [Failure and recovery](#failure-and-recovery) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-006` | [Workflow Sync reference warnings](#workflow-sync-reference-warnings) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-007` | [Local overrides](#local-overrides) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-008` | [Name conflicts](#name-conflicts), [HTTP API](#http-api), [Frontend](#frontend) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-009` | [Lossless switching](#lossless-switching) |
| `REQ-INTEGRATIONS-PROMPT-SYNC-010` | [Agent access](#agent-access) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `internal/reposync` (new, extracted) | Repository target validation (`Target.Normalize`), provider-neutral reads (`Reader.List`, `Reader.Getter`) through `GitHubClientProvider`/`GitLabClientProvider`, `ClassifyError`, `SafeErrorMessage`, and `BuildFailureDirective` (backoff or suspension). Extracted from `workflowsync` without behavior changes. |
| `internal/workflowsync` | Delegates listing, classification, sanitizing, and the failure directive to `reposync`. Adds the unresolved-reference warnings. Its automatic continuation and admission-deferral logic stay where they are. |
| `internal/prompts/store` | Adds `source` and `source_path` columns, ownership-guarded operator and agent writes, and the sync write methods. |
| `internal/prompts/service` | Adds `ErrPromptSyncManaged`, read-only guards, `ApplySyncedPrompts`, `ReleaseSyncedPrompts`, `UnresolvedPromptReferences`, and `ReferencedPromptNames`. |
| `internal/promptsync` (new) | Single-row config store, service (config, sync run, status), poller, and HTTP handlers. |
| `internal/workflow` repository | Adds `ListPromptReferenceTexts`, which returns every workflow prompt and step prompt in the installation for the deletion guard. |
| `internal/backendapp` | Wires the store, service, poller, and routes. Registers the store descriptor and its conformance adapter. |
| Web: `components/settings/prompt-sync-*` | Prompt Sync card, dialog, and status on Settings > Prompts. |
| Web: prompt list rows | Synced badge, with edit and delete hidden for synced prompts. |

## Data and contracts

### Prompt ownership

`models.Prompt` gains `Source string` (`"manual"` or `"sync"`) and
`SourcePath string`, the repository file path. Built-in prompts stay `manual`;
`builtin` is unchanged. A prompt is synced exactly when `Source == "sync"`.

Guards (AC-INTEGRATIONS-PROMPT-SYNC-004.1):

- `Service.UpdatePromptAtRevision` and `Service.DeletePrompt` return
  `ErrPromptSyncManaged` for a synced prompt. Handlers map it to HTTP `409`
  with the message "This prompt is managed by Prompt Sync and is read-only;
  edit its file in the synced repository." (backend diagnostic text; the web
  UI renders its own localized copy).
- `updateAgentPrompt` rejects synced prompts with `ErrPromptSyncManaged`. MCP
  maps it to a forbidden error, like a built-in prompt.
- Store operator update, agent update, and delete statements also guard on
  ownership (see [Local overrides](#local-overrides)), so a prompt that
  becomes synced after the service's read is not written.
- DTOs (`PromptDTO`, MCP summaries and reads, boot payload) add `source`
  and `source_path`.

The sync-only methods are `ApplySyncedPrompts(ctx, SyncApplyInput{Desired,
FrozenNames, ReferenceTexts})` and `ReleaseSyncedPrompts(ctx)`. They are not reachable from
HTTP, WS, or MCP.

### Local overrides

An override uses `source='override'` and keeps the prompt ID and name.
Operator and agent write guards use `source <> 'sync'`, so overrides
follow the manual editing rules.

- Ownership transitions in `store/ownership_writes.go` guard on row ID
  and source. Adoption also checks the name and content that sync read.
- `Service.SavePromptVersions` rejects an unlinked prompt
  (`ErrPromptNotLinked`), a stale `ExpectedUpdatedAt`, or invalid local
  text (`ErrInvalidPrompt`). `nextVersions` uses the existing text when
  a local copy is absent. The store updates source, both versions, and
  conflict in one statement.
  The statement checks the prior source and both versions to guard against
  concurrent changes. A guard miss returns `ErrPromptWriteRejected`.
- `promptsync.Service.SaveVersions` uses the service lock.
  `PUT /api/v1/prompt-sync/prompts/:id/versions` requires configuration
  management. It takes `{use_local, local_content?, expected_updated_at?}`.
  It returns 400 for invalid input, 404 for a missing prompt, and 409 for
  a prompt that cannot accept the choice. A Repository save uses the stored
  version. The next sync refreshes it.
- Prompt DTOs include `alternate_content` so the editor can preview both
  versions. Sync reports matching local overrides in `Overridden`. Their
  local text survives removal and participates in reference checks.

### Name conflicts

A conflict is a desired file whose name an existing prompt uses while sync
does not own it: a manual or built-in prompt, or an override whose `conflict`
flag is set. Built-in prompts follow exactly the manual rules, except that
sync never deletes them.

- `SyncApplyInput.Resolutions` maps a prompt name to `ChoiceLocal` or
  `ChoiceRepository`. `ApplySyncedPrompts` applies a clash as follows:

  | Existing prompt | Choice | Result |
  | --- | --- | --- |
  | manual or built-in | none | `AdoptPromptAsOverride(conflict=true)` with the file as repository version; reported in `Conflicts` and `Overridden` |
  | manual or built-in | local | `AdoptPromptAsOverride(conflict=false)` with the file as repository version |
  | manual or built-in | repository | `AdoptPromptAsSynced`: file content, old content kept in `alternate_content` |
  | conflicted override | none | unchanged; reported in `Conflicts` |
  | conflicted override | local | `ClearOverrideConflict` |
  | conflicted override | repository | `RevertOverride`, then `UpdateSyncedPrompt` with the file |

- `promptsync.Service.Check(ctx, req)` normalizes and authorizes a candidate
  request (or, with `nil`, loads and authorizes the stored configuration),
  binds the GitHub host, fetches and parses the files, and returns
  `CheckResult{Conflicts, Warnings}`. Each `Conflict` carries the name, file
  path, local content, and repository content. It writes nothing.
- `promptsync.Service.SaveAndSync(ctx, req)` normalizes and authorizes the
  request, then holds the service lock across the upsert and one manual sync
  with `req.Resolutions`. A failed sync is returned in `SaveOutcome.SyncErr`
  with the saved configuration. `SyncWithResolutions` runs Sync now with
  choices; plain `Sync` passes none. Resolutions are validated
  (`local` or `repository`) and never stored.
- Identical text is no conflict (AC-INTEGRATIONS-PROMPT-SYNC-008.8): when a
  manual, built-in, or conflicted override prompt has the file's content,
  `applySyncedFile` treats the choice as `ChoiceRepository`, and
  `findConflicts` skips it.
- When an override's file leaves the source, the vanished-file pass calls
  `DetachOverride` (AC-INTEGRATIONS-PROMPT-SYNC-008.7). The guarded update
  clears `conflict`, `alternate_content`, and `source_path` on the same row.
  The existing detach guard and revision update apply to this write.
- `SyncResult` adds `conflicts` and `restored`. A run that leaves a conflict
  is not `unchanged`.

### Lossless switching

`custom_prompts.alternate_content` holds the version a prompt does not use
now: the kept local version of a synced prompt, or the repository version of
an override. Empty means none; prompt content is never empty after trimming,
so the empty string is unambiguous.

- `SavePromptVersions` writes both versions together. A first override
  starts from the synced content and keeps it as the repository version;
  choosing the repository version uses the stored one at once and keeps the
  local content, including an edit made while the switch was on.
  `RevertOverride` swaps the two versions in one statement: `content =
  COALESCE(NULLIF(alternate_content, ''), content), alternate_content =
  content` (SET reads the pre-update row on SQLite and PostgreSQL).
  `AdoptPromptAsSynced` keeps the old content; `AdoptPromptAsOverride`
  stores the file's content.
- An override with empty `alternate_content` (file left the source) has no
  repository version: `SavePromptVersions` rejects Repository with
  `ErrNoRepositoryVersion` (409), and the row shows "Removed from repository"
  instead of the switch (`hasRepositoryVersion`). The HTTP, boot payload,
  and MCP projections return an empty path for such an override, including
  older detached rows that still store an old path.
- `UpdateSyncedPrompt` leaves `alternate_content` alone. For every override
  with a valid matching file, `ApplySyncedPrompts` records its repository
  version and path in one guarded update (AC-INTEGRATIONS-PROMPT-SYNC-009.3).
  A path-only change is sufficient to write and publish a prompt update.
  The repository refresh uses the existing revision policy for local edits.
  Invalid and duplicate names follow the existing freeze rules.
- In the vanished-file pass, a synced prompt with a kept copy, or a built-in
  prompt, goes through `RestoreSyncedPrompt` (back to `manual`, kept copy
  restored) instead of the reference check and `DeleteSyncedPrompt`. It is
  reported in `Restored`. `DeleteSyncedPrompt` also guards `builtin = ?` with
  `0`.
- Removal (AC-INTEGRATIONS-PROMPT-SYNC-009.5, 009.6): `RemovalReview` lists
  every synced prompt or override whose `alternate_content` is set and
  differs from `content`, as `Conflict` entries with `in_use` (`repository`
  for synced, `local` for overrides), and the names of synced, non-built-in
  prompts with no `alternate_content` (`repository_only`).
  `GET /api/v1/prompt-sync/kept-versions` returns `{prompts,
  repository_only}`. `DELETE /config` accepts `{"choices": {name:
  local|repository}, "delete_repository_only": bool}`; `DeleteConfig`
  validates the choices, names the prompts whose choice differs from the
  version in use, and passes a `ReleaseOptions{UseOther,
  DeleteRepositoryOnly}` to `ReleaseSyncedPrompts`. In one transaction it
  sets `content = alternate_content` for the `UseOther` rows, deletes the
  named `delete_repository_only` rows that still match `source = 'sync' AND
  alternate_content = '' AND builtin = 0`, and releases the rest, advancing
  `updated_at`. `authorizeRemoval` skips the check for a deleted workspace.

### Agent access

- `updateAgentPrompt` checks synced ownership before the built-in check and
  returns `ErrPromptSyncManagedForAgent`. Its message says sync owns the
  prompt, that a local change has no effect, that the user can switch it to
  Local in Settings > Prompts and save, and that agents cannot change that
  setting.
  The error also matches `ErrPromptSyncManaged`. MCP maps it to forbidden.
- MCP list and read results add `conflict` and `edit_guidance` (from
  `promptdto.EditGuidance`) for synced prompts and overrides. Agent writes to
  overrides use the agent write guard, which leaves `conflict` unchanged.
  For a detached override, guidance identifies the local text in use and
  explains that the file is absent.
- No MCP tool overrides, reverts, or resolves. The `update_shared_prompt`
  and `list_shared_prompts` tool descriptions say that only the user switches
  a prompt to Local. A server test asserts that no tool name on any surface
  mentions override, revert, or prompt sync.
- The HTTP routes cannot tell an agent holding the user's own credential from
  the user; they keep the configuration-management requirement.
- Prompt DTOs and MCP reads report `source: "override"`;
  `allow_agent_edits` is true for overrides.

### Reference helpers

`UnresolvedPromptReferences(ctx, texts) ([][]string, error)` returns, per
text, the distinct reference tokens that resolve to no saved prompt. A token starts at
an `@` that passes `isPromptReferenceStart` and is the longest run of
characters that pass `isPromptReferenceNameCharAt`. It is unresolved when
`matchPromptReference` finds no prompt at that position, using the full
current prompt set. This uses the same rules and candidate limits as
`ResolvePromptReferences`. A text with no `@` returns quickly.

`ReferencedPromptNames(ctx, texts, names) (map[string]int, error)` counts,
for each name in `names`, how many distinct owners (workflow IDs) have a text
containing a reference that
`matchPromptReference` resolves to that name against the full prompt set.
Longest-match semantics therefore hold: `@review-strict` counts toward
`review-strict`, not `review`.

### Prompt Sync configuration

Table `prompt_sync_config`, with at most one row (`id = 'default'`):

| Column | Meaning |
| --- | --- |
| `provider` | `github` or `gitlab` |
| `workspace_id` | Credential workspace |
| `github_host`, `repo_owner`, `repo_name` | GitHub source |
| `project_path` | GitLab source |
| `branch`, `path`, `interval_seconds`, `poll_enabled` | Same normalization as `workflowsync.SetConfigRequest`; `path` defaults to `.kandev/prompts` |
| `last_synced_at`, `last_ok`, `last_error`, `last_warnings`, `last_hash` | Status |
| `consecutive_failures`, `next_attempt_at`, `last_error_class`, `poll_suspended`, `poll_suspension_reason`, `credential_fingerprint` | Recovery state |
| `created_at`, `updated_at` | Timestamps |

Request normalization reuses the Workflow Sync validators, moved into
`reposync` as `NormalizeSource`, with the default path supplied by the
caller.

### HTTP API

| Method | Route | Behavior |
| --- | --- | --- |
| `GET` | `/api/v1/prompt-sync/config` | `200` config + status, or `204` |
| `POST` | `/api/v1/prompt-sync/check` | Read a candidate source (request body) or the stored one (empty body) and return `{conflicts, warnings}`. Writes nothing. `422` with a sanitized `error` when the source cannot be read. |
| `POST` | `/api/v1/prompt-sync/config` | Validate, upsert, then run one sync with the body's `resolutions`. Returns `{config, result}` or `{config, error}`. |
| `GET` | `/api/v1/prompt-sync/kept-versions` | Synced prompts and overrides with two different versions: `{prompts: [{name, path, local_content, repository_content, in_use}]}`. |
| `DELETE` | `/api/v1/prompt-sync/config` | Release synced prompts (optional `{"choices": {name: "local"\|"repository"}}` picks the version each keeps), then delete. Returns `{"deleted":true}`; `400` for an unknown choice. |
| `POST` | `/api/v1/prompt-sync/sync` | Run now, with optional `{"resolutions": {...}}`. Returns `{config, result}` or `{config, error}`. `404` when not configured; `400` for an unknown choice. |

`result` uses the Workflow Sync shape: `created`, `updated`, `deleted`,
`warnings`, `unchanged`. A completed run returns `200` even when it carries
`error`, matching Workflow Sync.

## Control flow

One service mutex serializes runs, version saves, configuration writes,
and removal. **Sync now** and removal authorize the loaded configuration
under that mutex. Sync uses the authorized snapshot for fetch and apply.

1. Load the configuration. For automatic sync, check the schedule and
   suspension status.
2. Check the credential workspace and bind the GitHub connection if used.
3. List and read the files through `reposync.Reader`. Parse the files into
   desired versions, frozen names, and warnings.
4. Read workflow references. Apply the desired versions through
   `ApplySyncedPrompts`. Check remaining synced prompts for unresolved
   references. For GitHub, `CommitWorkspaceGitHubHost` guards this step
   against connection replacement.
5. Record status and return the result.

Automatic GitHub runs use non-blocking background admission. A deferred
run leaves failure status intact and retries on the next tick.

Manual GitHub runs opt in to `WithNonBlockingGitHubAdmission` in
`syncLocked` (AC-INTEGRATIONS-PROMPT-SYNC-001.6). An opted-in interactive
request returns `AdmissionWaitError` when quota blocks it. This applies to
the listing and each download. The error carries the retry boundary and
distinguishes a quota block from cancellation. An admitted request uses
its normal transport deadline. The source check fetches outside the lock.

### Prompt file parsing

- Selection: `entry.isFile && strings.EqualFold(filepath.Ext(name), ".md")`.
- Name: `strings.TrimSuffix` of the case-insensitive `.md` extension. It is
  not trimmed further, so a name with surrounding whitespace is invalid
  (AC-INTEGRATIONS-PROMPT-SYNC-002.3).
- Content: `utf8.Valid` and no NUL byte, then `strings.TrimSpace`, then the
  saved-prompt limits (name ≤ 512 bytes, content ≤ 1 MiB). Provider reads
  share one oversize contract: GitLab returns `gitlab.ErrRepoFileTooLarge`
  above 1 MiB and GitHub returns `github.ErrRepoFileTooLarge` when the
  contents API reports `encoding: none`. Workflow Sync keeps treating either as
  a read failure.
- Files are grouped by derived name before content validation. A name claimed
  by two or more files is frozen and every file that produced it is dropped,
  even if one of them is invalid. An invalid single file adds its name to
  `frozenNames`.
- A read that fails with `reposync.IsFileTooLarge` (GitLab
  `ErrRepoFileTooLarge` over 1 MiB; GitHub `ErrRepoFileTooLarge` for the
  contents API's `encoding: none`) is recorded as a too-large file and frozen
  with a warning instead of failing the run.

### Reconciliation

`ApplySyncedPrompts` runs in the prompt store with these per-name rules.
Statements are individual, and each one is guarded by the expected ownership
in its `WHERE` clause:

| Existing row | In desired | Action |
| --- | --- | --- |
| none | yes | Insert with `source='sync'`, `source_path`. A unique conflict (a concurrent manual create) becomes a clash warning. |
| sync | yes, content or path differs | `UPDATE … WHERE id=? AND source='sync'` |
| sync | yes, identical | none |
| manual or built-in | yes | Conflict (see [Name conflicts](#name-conflicts)) |
| sync | no, name frozen | none |
| sync, with a kept copy or built-in | no | `RestoreSyncedPrompt` to manual with the kept copy |
| override with conflict | no | Clear the conflict |
| sync | no | Referenced by a workflow text or a surviving saved prompt (re-read after this run's writes; followed to a fixpoint through prompts kept this way): keep and warn; else `DELETE … WHERE id=? AND source='sync' AND builtin=0` |

The method returns sorted `created`, `updated`, `deleted`, and `warnings`. It
publishes the existing `prompts changed` event once when anything changed,
including when a later step returns an error after earlier writes were saved,
which drives AC-INTEGRATIONS-PROMPT-SYNC-003.6 through the current WS
refresh path. Removal releases prompts before deleting the configuration, in two
statements owned by different stores. If the delete fails after the release,
the configuration remains while its prompts are already manual; later syncs
adopt those files as conflicted overrides instead of updating them. The caller
receives the error, and a retried removal releases nothing and deletes the
configuration, so the state recovers without losing prompt content.

### Workflow Sync reference warnings

The Workflow Sync Applier gains no new method. After
`ApplySyncedWorkflows`, `workflowsync.applyFetchedFiles` calls a narrow
`PromptReferenceChecker` interface (satisfied by the prompt service). It
checks each parsed export's workflow prompt and step prompts and groups
missing names by (file, workflow, name). Each group becomes one warning:
`<path>: workflow "<name>" references @<missing> in its workflow prompt and
step(s) "<a>", "<b>", which is not a saved prompt`, naming only the places
that apply. A nil checker skips the check. A checker error
adds one warning and never fails the run. With no missing names the warning
list, the `unchanged` flag, and the stored status match the current output
(AC-INTEGRATIONS-PROMPT-SYNC-006.2).

## Failure and recovery

- Listing and download errors fail the run before apply and record a failure
  through `reposync.BuildFailureDirective`. This is the same backoff, GitHub
  suspension, and GitLab circuit policy Workflow Sync uses
  (AC-INTEGRATIONS-PROMPT-SYNC-005.2 and 005.3).
- Before any provider call, the run checks that the credential workspace
  exists. A missing workspace records `ErrCredentialWorkspaceMissing` and
  suspends polling for either provider; a missing connection surfaces as the
  provider's not-configured error and follows the provider policy.
- Saving the config clears recovery state. **Sync now** bypasses the
  schedule. A changed `WorkspaceConnectionFingerprint` resets the circuit on
  the next tick.
- Errors returned by apply (for example, a prompt repository failure) record
  a failure. Prompt rows already written by that run stay written. The next
  run reconciles them, and because each write is guarded by ownership, a
  partial run cannot touch manual prompts.
- Prompt Sync sanitizes stored errors through `safeErrorMessage`.
  This helper delegates to `reposync.SafeErrorMessage` for provider errors.
- A local interactive GitHub quota block ends the fetch before apply.
  The transient retry policy uses `AdmissionWaitError.RetryAt`.
  Prompt Sync reuses `github.RateLimitErrorCode` in the sync response's
  `error_code` and the existing `last_error_class` status field.
  The HTTP 200 envelope carries the configuration and a safe retry message.

## Persistence

- Count-limited inserts (manual and synced creation) run under one
  transaction-scoped `pg_advisory_xact_lock` on PostgreSQL, so concurrent
  creates cannot both pass the 2,000-prompt limit. SQLite's single writer
  already serializes the statement.
- `custom_prompts` gains `source TEXT NOT NULL DEFAULT 'manual'`,
  `source_path TEXT NOT NULL DEFAULT ''`, `alternate_content TEXT NOT NULL
  DEFAULT ''`, and `conflict INTEGER NOT NULL DEFAULT 0`. These are idempotent `ADD COLUMN`
  migrations in the prompt store's `initSchema`, run after `CREATE TABLE`,
  on SQLite and PostgreSQL. Existing rows become `manual`, so upgrades keep
  every prompt editable.
- `prompt_sync_config` is created by `promptsync.NewStore` with
  `CREATE TABLE IF NOT EXISTS`, using the `{{boolean}}` dialect templating
  Workflow Sync uses. It is registered in `internal/persistence/requiredstores`
  with a fixed adapter in `internal/persistence/storeconformance`.
- E2E reset deletes the workspace's `prompt_sync_config` row and, once no
  configuration remains, the non-built-in synced prompts it created, and
  returns adopted built-in prompts to manual with their own content.
- Deleting a workspace leaves the config in place. The next run fails and
  suspends with an actionable reason (AC-INTEGRATIONS-PROMPT-SYNC-005.3).

## Security

- Mutating routes mount `authz.RequireOrgScope(authz.ScopeOrgConfigManage)`.
  The service also calls the task service's `AuthorizeWorkspaceAccess` for
  the configured workspace (on save, the requested one) before the save,
  delete, or sync, so a user cannot borrow a credential from a workspace they
  cannot see. `GET` uses the same scope as prompt reads.
- Automatic runs carry no identity and are unscoped, like the Workflow Sync
  poller.
- No credential is stored. Repository content is untrusted text: it is stored
  as prompt content and expanded only through the existing
  `sysprompt.StripTags` sanitization.

## Frontend

- Types are in `lib/types/prompt-sync.ts` and `lib/types/http.ts`. Requests
  use `lib/api/domains/prompt-sync-api.ts`. Both sync features share
  `repo-sync-form.ts`, `RepoSyncFields`, the removal controls, and
  `WorkflowSyncStatusCard`.
- `hooks/domains/settings/use-prompt-sync.ts` owns the configuration, draft,
  and actions. `prompt-sync-section.tsx` is a settings group above the prompt
  list: **Set up** when unconfigured, otherwise the shared status card with
  **Configure** and **Sync now**. `prompt-sync-dialog.tsx`
  holds a workspace select (credential workspace) plus the shared fields, and
  **Save** and **Remove sync** with inline or mobile confirmation.
- Prompt Sync translates the GitHub rate-limit message in the existing
  toast and status card. The same **Sync now** action retries on desktop
  and phone.
- `PromptVersionControl` shares `VersionSwitch` with the review step.
  It is the first trailing control and has a 44px coarse-pointer target.
  The Repository choice uses the read-only `PromptRepositoryView` and
  the file note. The Local choice uses `PromptEditForm`. A version choice
  preserves each row's open or closed state. The list displays ownership,
  conflict, and unsaved badges as applicable.
- `use-prompts-page.ts` keys open rows, drafts, and pending choices by
  prompt ID. `planPromptSaves` sends `savePromptVersions` for a changed
  linked prompt and `updatePrompt` for a changed manual prompt. Each
  write carries the draft's starting revision. A successful write closes
  the row and clears its draft. Reset clears pending edits and closes
  their rows.
  `PromptOrphanedDraftsNotice` lists drafts whose rows disappeared. Save
  recreates those prompts.
- For a detached override, the badge tooltip describes the remaining local
  version. The inline "Removed from repository" status and Edit control give
  the outcome on desktop and phone without opening the tooltip.
- The prompt editor uses the dark Monaco theme in every app theme. Read-only
  editors (`SettingsPromptEditor` with `readOnly`) set `data-read-only` and
  a dashed frame; `app/globals.css` lifts their surface to a lighter dark
  tone (text contrast above 4.5:1) and hides the cursor.
- `PromptSyncSection` renders its group with `frame="none"`: only the
  heading and **Set up** while unconfigured, then the status card as the
  single outline.
- `PromptConflictReviewList` (`prompt-conflict-review.tsx`) explains the
  local and repository versions once in a legend, then renders one compact
  row per prompt: name and path, a `Local | switch | Repository` control
  (switch on is the repository version, accessible name "Use the repository
  version of @name"), and an icon button that shows both versions below the
  row. A `kind` prop (`conflict` or `removal`) picks the legend copy.
- Save in `prompt-sync-dialog.tsx` runs `usePromptSync.handleSave`: a
  `checkPromptSync(payload)` first; conflicts open `usePromptConflictReview`
  (each row starting on local) and the dialog swaps its fields for the review
  list with **Back**. Saving from the review step posts the choices as
  `resolutions`. When the save's sync reports conflicts the check did not
  list, the hook checks the stored configuration and reopens the review
  instead of closing.
- **Remove sync** first calls `usePromptSync.startRemoval`, which reads the
  removal review. With two-version or repository-only prompts, the dialog
  shows a removal step (`PromptRemovalReview` in `prompt-removal-review.tsx`):
  the review list with removal copy when there are two-version prompts, each
  row starting on its `in_use` version; a checkbox, off by default, naming the
  repository-only prompts; **Back**; and a destructive **Remove**.
  `handleDelete` sends `{choices, delete_repository_only}`. The removal state
  (`useRemovalReview`) resets the checkbox when the step closes. With neither,
  the existing inline or mobile confirmation opens.
- `PromptConflictNotice` renders first on Settings > Prompts, above the help
  text: an amber `role="alert"` banner with a count title, the conflicting
  prompt names, and a primary **Review conflicts** button, which opens
  `PromptConflictReviewDialog` through `usePromptConflictResolver` (check the
  stored configuration, then Sync now with the choices). The Prompt Sync card
  refreshes its own status.
- `PromptConflictIndicator` (count from `usePromptConflictCount`, which loads
  the prompt list) marks the Prompts row of the settings menu, the sidebar
  Settings button, and the Settings row of phone navigation.
- All copy goes through `t()` in the `settings` namespace, in seven locales.

## Observability

- Expvar counters `prompt_sync_runs_total` with closed keys `success` and
  `failure`, and `prompt_sync_poll_skipped_total` with the closed keys
  `suspended` and `rate_limited` (deferred background run). They carry no
  repository, workspace, or prompt labels.
- Structured zap logs: `applied prompt sync` (with counts),
  `prompt sync attempt failed`, `prompt sync polling suspended`, and
  `prompt sync polling recovered`.

## Related decisions

- [Workflow Sync public guide](../../../public/workflow-sync.md): the
  reconciliation vocabulary this design mirrors.

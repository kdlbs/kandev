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
- Store writes add `AND source = 'manual'` to the operator update, agent
  update, and delete statements. This closes the race in which a prompt
  becomes synced between the service's read and the write; the write then
  affects no row and an update reports the existing stale-write rejection.
- DTOs (`PromptDTO`, MCP summaries and reads, boot payload) add
  `source: "manual" | "sync"` and `source_path`. `allow_agent_edits` becomes
  `!builtin && source == "manual"`.

The sync-only methods are `ApplySyncedPrompts(ctx, SyncApplyInput{Desired,
FrozenNames, ReferenceTexts})` and `ReleaseSyncedPrompts(ctx)`. They are not reachable from
HTTP, WS, or MCP.

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
| `POST` | `/api/v1/prompt-sync/config` | Validate and upsert. Does not sync. |
| `DELETE` | `/api/v1/prompt-sync/config` | Release synced prompts, then delete. Returns `{"deleted":true}`. |
| `POST` | `/api/v1/prompt-sync/sync` | Run now. Returns `{config, result}` or `{config, error}`. `404` when not configured. |

`result` uses the Workflow Sync shape: `created`, `updated`, `deleted`,
`warnings`, `unchanged`. A completed run returns `200` even when it carries
`error`, matching Workflow Sync.

## Control flow

```text
POST /prompt-sync/sync ─┐
Poller (60s tick) ──────┼─► promptsync.Service.Sync (install lock)
                        │     1. load config, check due/suspended (automatic only)
                        │     1a. credential workspace must exist, else suspend
                        │     2. bind GitHub host for workspace_id (GitHub)
                        │     3. reposync.List(source) → keep *.md immediate files
                        │     4. reposync.Get each file (any error → fail run)
                        │     5. parse files → desired{name→content,path}, frozen names, warnings
                        │     6. workflow repo ListPromptReferenceTexts → texts
                        │     7. prompts.ApplySyncedPrompts(desired, frozen, texts)
                        │     8. prompts.UnresolvedPromptReferences per synced prompt
                        └─►   9. record status, return result
```

One process-wide mutex serializes sync runs, config saves, and removal,
mirroring the per-workspace lock in Workflow Sync. The GitHub apply step runs
inside `CommitWorkspaceGitHubHost`, so files fetched under a replaced
connection cannot be applied. Automatic runs use the GitHub background work
class with blocking admission; with only one source, no continuation or
deferral queue is needed.

### Prompt file parsing

- Selection: `entry.isFile && strings.EqualFold(filepath.Ext(name), ".md")`.
- Name: `strings.TrimSuffix` of the case-insensitive `.md` extension. It is
  not trimmed further, so a name with surrounding whitespace is invalid
  (AC-INTEGRATIONS-PROMPT-SYNC-002.3).
- Content: `utf8.Valid` check, then `strings.TrimSpace`, then the
  saved-prompt limits (name ≤ 512 bytes, content ≤ 1 MiB). Provider reads
  already cap a file at 1 MiB plus one byte.
- An invalid file adds its derived name (when non-empty) to `frozenNames`.
  Duplicate names add the name to `frozenNames` and drop every file that
  produced it.

### Reconciliation

`ApplySyncedPrompts` runs in the prompt store with these per-name rules.
Statements are individual, and each one is guarded by the expected ownership
in its `WHERE` clause:

| Existing row | In desired | Action |
| --- | --- | --- |
| none | yes | Insert with `source='sync'`, `source_path`. A unique conflict (a concurrent manual create) becomes a clash warning. |
| sync | yes, content or path differs | `UPDATE … WHERE id=? AND source='sync'` |
| sync | yes, identical | none |
| manual or built-in | yes | Skip and warn (AC-INTEGRATIONS-PROMPT-SYNC-003.3) |
| sync | no, name frozen | none |
| sync | no | `ReferencedPromptNames`: if count > 0, keep and warn; else `DELETE … WHERE id=? AND source='sync'` |

The method returns sorted `created`, `updated`, `deleted`, and `warnings`. It
publishes the existing `prompts changed` event once when anything changed,
which drives AC-INTEGRATIONS-PROMPT-SYNC-003.6 through the current WS
refresh path. `ReleaseSyncedPrompts` runs one statement,
`UPDATE custom_prompts SET source='manual', source_path='' WHERE
source='sync'`, and publishes once when any row changed.

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
- All stored errors pass through `reposync.SafeErrorMessage`.

## Persistence

- `custom_prompts` gains `source TEXT NOT NULL DEFAULT 'manual'` and
  `source_path TEXT NOT NULL DEFAULT ''`. These are idempotent `ADD COLUMN`
  migrations in the prompt store's `initSchema`, run after `CREATE TABLE`,
  on SQLite and PostgreSQL. Existing rows become `manual`, so upgrades keep
  every prompt editable.
- `prompt_sync_config` is created by `promptsync.NewStore` with
  `CREATE TABLE IF NOT EXISTS`, using the `{{boolean}}` dialect templating
  Workflow Sync uses. It is registered in `internal/persistence/requiredstores`
  with a fixed adapter in `internal/persistence/storeconformance`.
- E2E reset deletes the workspace's `prompt_sync_config` row and, once no
  configuration remains, the synced prompts it created.
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

- `lib/types/http.ts`: `CustomPrompt` gains `source` and `source_path`. A new
  `lib/types/prompt-sync.ts` and API client functions in
  `lib/api/domains/settings-api.ts` cover the four routes.
- Pure form logic shared by Workflow Sync and Prompt Sync (link parsing,
  provider switching, payloads, validation) lives in
  `lib/settings/repo-sync-form.ts`. The fields (provider tabs, repository
  link, branch and directory, polling) live in
  `components/settings/repo-sync-fields.tsx` and the removal confirmation in
  `repo-sync-removal-actions.tsx`, both parameterized by a test-ID prefix.
  `WorkflowSyncStatusCard` takes `testIdPrefix` and extra `actions`, and
  Prompt Sync reuses it. Workflow Sync renders identically.
- `hooks/domains/settings/use-prompt-sync.ts` owns the configuration, draft,
  and actions. `prompt-sync-section.tsx` is a settings group above the prompt
  list: **Set up** when unconfigured, otherwise the shared status card with
  **Configure** and **Sync now**. `prompt-sync-dialog.tsx`
  holds a workspace select (credential workspace) plus the shared fields, and
  **Save** and **Remove sync** with inline or mobile confirmation.
- Prompt rows: `prompt.source === "sync"` renders a `PromptSyncedBadge`
  (tooltip: source path) and hides `PromptRowActions`.
- All copy goes through `t()` in the `settings` namespace, in seven locales.

## Observability

- Expvar counters `prompt_sync_runs_total` with closed keys `success` and
  `failure`, and `prompt_sync_poll_skipped_total` with the closed key
  `suspended`. They carry no repository, workspace, or
  prompt labels.
- Structured zap logs: `applied prompt sync` (with counts),
  `prompt sync attempt failed`, `prompt sync polling suspended`, and
  `prompt sync polling recovered`.

## Related decisions

- [Workflow Sync public guide](../../../public/workflow-sync.md): the
  reconciliation vocabulary this design mirrors.

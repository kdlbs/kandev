---
created: 2026-11-21
status: complete
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-002
  - REQ-INTEGRATIONS-PROMPT-SYNC-005
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
  - REQ-INTEGRATIONS-PROMPT-SYNC-008
  - REQ-INTEGRATIONS-PROMPT-SYNC-009
  - REQ-INTEGRATIONS-PROMPT-SYNC-010
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
legacy_specs: []
---

# Implementation plan for Prompt Sync follow-ups

## Overview

Repair two defects from [issue #5416](https://github.com/kdlbs/kandev/issues/5416) on
main at `018808f8d949`, the merge of
[PR #5394](https://github.com/kdlbs/kandev/pull/5394). Deliver the quota
fix first to keep Save available during a quota wait. Then repair the
override path exposed to Settings and agents.

The [Prompt Sync requirements](../../specs/integrations/requirements/prompt-sync.md)
and the [local version requirements](../../specs/integrations/requirements/prompt-sync-local-versions.md)
cover both outcomes. The [Prompt Sync design](../../specs/integrations/system-design/prompt-sync.md)
defines the provider and storage boundaries. The [original delivery plan](../prompt-sync/plan.md)
records the shipped feature and its original verification.

## Scope

- Item 4. End a manual GitHub sync when the coordinator would wait for
  quota. Show the reason in the existing Settings error display.
- Item 2. Clear the path of a detached override. Restore the current path
  and repository version together if a valid file with the same name returns.
- Item 5. Align AC-INTEGRATIONS-PROMPT-SYNC-007.2 with the shipped version
  Save behavior. Save uses the stored repository version. The next sync
  refreshes it.

### Other issue findings

| Item | Finding and disposition |
| --- | --- |
| 1. Version columns | Prompt validation rejects empty local and repository text. Empty alternate content represents the absence of a version in the shipped model. Splitting columns needs a separate schema and HTTP/MCP compatibility decision for every existing state on SQLite and PostgreSQL. |
| 2. Removed override | AC-INTEGRATIONS-PROMPT-SYNC-007.3 and 009.1 keep the local override editable. Work order 02 repairs its path and guidance. Releasing it to manual ownership requires a product decision. |
| 3. Rename semantics | `TestRenamedOverrideFileKeepsOldOverrideAndCreatesSyncedPrompt` passes. The old name keeps its local override and the new name gets the repository version. Name-based reconciliation receives a directory snapshot. Rename conflicts or reference warnings need a policy for ambiguous add and remove pairs. |
| 4. Manual wait | Local diagnostics show that interactive admission waits on quota and that version Save shares the held lock. Work order 01 covers the quota wait. |
| 5. Repository version age | `last_synced_at` belongs to the configuration and also changes on a failed attempt. A frozen file can retain older text across a successful sync. A per-row age would need evidence of the last successful read for that file. |
| 6. Background deferral | The local coordinator defers a file download directly after a successful listing due to background pacing. Another tick can list and defer again. Address forward progress before adding a wait indicator or using `RetryAt` to skip more ticks. The separate design needs resumable reads tied to the source and credential, or a guarded fetch outside the service lock. It also needs to distinguish routine pacing from a quota pause that the operator can act on. |
| 7. Removal authorization | The route requires `org.config.manage`. The service bypasses its workspace check only when lookup confirms deletion. AC-INTEGRATIONS-PROMPT-SYNC-001.4 describes the ordinary workspace check. Decide the authority for removing the install-wide configuration after its workspace is gone before reconciling the requirement. |
| 8. Test and CI follow-ups | The focused SQLite cancellation test and both recovery browser tests passed on current main. The original parallel backend run and CI shard remain unverified. The local pacing test used the real coordinator with an HTTP fixture. The multi-row E2E test checks saving Repository. Only the service test covers a newer upstream version that has yet to sync. |
| 9. Direction sign-off | [Issue #5391](https://github.com/kdlbs/kandev/issues/5391) is the original issue. A [repository member's reply](https://github.com/kdlbs/kandev/issues/5391#issuecomment-7046068138) supports the local override direction. [PR #5394](https://github.com/kdlbs/kandev/pull/5394) references and closed it. |

## Technical approach

### Manual quota wait

`SyncWithResolutions` and `SaveAndSync` hold `Service.mu` across the
repository reads and apply. `SaveVersions` and `DeleteConfig` take that
same lock. `RateAdmission.acquireInteractive` can wait until GitHub's reset
or Retry-After boundary. The current non-blocking context option only affects
background calls.

Use the existing non-blocking admission option for manual sync under the
lock. Extend it to interactive requests and reuse the GitHub rate-limit
error code, retry boundary, and Settings feedback. The source check
already fetches outside the lock.

### Detached override path

`DetachOverride` clears `alternate_content` and `conflict` and leaves the
path populated. The HTTP, boot, and MCP projections expose that path. If a
valid file returns, the override refresh currently writes only the text.

Clear the path in the detach update. Pair path and text in the same update
for a valid matching file. Use the absence of a repository version to show
an empty path on reads of older detached overrides.

## ASCII UI preview

### UI-01. Quota-blocked manual sync in Settings > Prompts

Before, **Sync now** can stay busy until the quota wait ends. After the
repair, both viewports show timing guidance and the existing retry action.

```text
Desktop
Prompt Sync | acme/prompts                    [Configure] [Sync now]
GitHub quota is unavailable. Try again after quota resets.

Phone
Prompt Sync
acme/prompts
[Configure] [Sync now]
GitHub quota is unavailable.
Try again after quota resets.
```

If Save stored the configuration before the block, the existing toast says
the configuration was saved and the sync failed. The phone actions keep
their touch-sized targets. Copy and line breaks are illustrative.
AC-INTEGRATIONS-PROMPT-SYNC-001.6 applies to this view.

### UI-02. Override after its file leaves the source

```text
Desktop
@hook [Overridden]                Removed from repository  [Edit] [Delete]
Badge tooltip: Your local version is still editable.

Phone
@hook [Overridden]
Removed from repository
[Edit] [Delete]
```

The phone shows the state inline and offers the existing Edit action.
When a valid same-name file returns, both viewports show the Local and
Repository switch again. The badge then names the current file.
AC-INTEGRATIONS-PROMPT-SYNC-007.2, 008.7, and 009.3 apply to this view.

## Tests

- AC-INTEGRATIONS-PROMPT-SYNC-001.6, 002.5, 005.2, and 005.4 use
  `TestRateCoordinatorOptInInteractiveAdmissionReturnsQuotaRetry`,
  `TestRateCoordinatorOptInInteractiveAdmissionUsesLatestActiveQuotaDeadline`,
  `TestRateCoordinatorOptInInteractiveAdmissionUsesSecondaryDeadlineWithHealthyPrimary`, and
  `TestPromptSyncManualQuotaDoesNotBlockVersionSave`. Route tests cover
  the shared sync response for Sync now and configuration Save.
- AC-INTEGRATIONS-PROMPT-SYNC-007.2, 008.7, 009.1, 009.3, and 010.2 use
  `TestDetachedOverrideClearsAndRestoresRepositoryPath` and
  `TestSharedPromptReadOmitsStaleRepositoryPathForDetachedOverride`.
  The existing stored-version test checks the version Save contract.
- Work order 02 exercises the guarded updates on SQLite. The PostgreSQL
  ownership test was skipped because `KANDEV_TEST_POSTGRES_DSN` was not set.
  Its older-row case covers the read of a detached override with a stale path.

## E2E tests

- In `tests/settings/prompt-sync.spec.ts` (chromium) and
  `tests/settings/mobile-prompt-sync.spec.ts` (mobile-chrome), show the quota
  message after the typed reply. Retry with **Sync now** and show success.
  This covers AC-INTEGRATIONS-PROMPT-SYNC-001.6 in the browser. The Go tests
  cover admission and the concurrent Save.
- In the same specs, remove and restore a file in the fixture repository.
  Verify **Removed from repository**, the editable local text, and the
  restored switch. This covers AC-INTEGRATIONS-PROMPT-SYNC-007.2, 008.7,
  and 009.3 through the real sync API.

## Work orders

- [x] [Task 01: Bound manual GitHub quota admission](task-01-bound-manual-quota-admission.md)
- [x] [Task 02: Repair detached override metadata](task-02-repair-override-metadata.md)

## Verification results

- Temporary Go fixtures reproduced the Save lock and stale path on main.
  A coordinator test confirmed the old non-blocking option ignores interactive
  quota waits. A PAT client and HTTP fixture confirmed two successful listings
  and zero file downloads due to background pacing.
- The existing Go tests for rename behavior, the stored repository version,
  deleted-workspace removal, and automatic deferral passed. The Chromium
  multi-row Save test passed and exercised the Repository choice in Settings.
- Focused SQLite cancellation test passed in 6.62 seconds. Both tests in the
  Playwright recovery spec passed in one Chromium worker with retries disabled.
- Catalog validation, specification lint, and whitespace checks passed.
  The documentation coverage preflight accepted the six design files
  and the projected implementation paths.
- PR review coverage confirms that an exhausted primary reset wins over a
  shorter secondary throttle, while a healthy primary reset does not override
  a secondary-only retry deadline.
- Desktop Chromium and mobile Chrome recovery tests reload after detachment and
  after file recovery, then verify the path, Local selection, and local content.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -timeout 45s -count=1 -run '^TestTaskMetadataMergeSQLiteCancellation$' -v ./internal/task/repository/sqlite)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/multi-repo-session-resume-recovery.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/settings/prompt-sync.spec.ts -- --grep 'switches and edits several prompts' --retries=0)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -run '^(TestRateCoordinatorOptInInteractiveAdmissionUsesLatestActiveQuotaDeadline|TestRateCoordinatorOptInInteractiveAdmissionUsesSecondaryDeadlineWithHealthyPrimary)$' ./internal/github)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/settings/prompt-sync.spec.ts -- --grep 'restores an override file after removal' --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-prompt-sync.spec.ts -- --grep 'restores an override file after removal' --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/prompt-sync-follow-up-fixes
git status --short --untracked-files=all -- docs/plans/prompt-sync-follow-up-fixes
```

Implementation results:

- Task 01 completed. Manual GitHub quota waits now return typed retry guidance
  without holding the Prompt Sync lock. Its backend, web, typecheck, i18n,
  desktop and mobile browser checks passed.
- Task 02 completed. Detach clears repository metadata, returning files restore
  their path and text atomically, and HTTP/boot/MCP reads hide stale detached
  paths. Its backend, web, typecheck, i18n, desktop and mobile browser checks
  passed. The PostgreSQL-specific ownership test was skipped because
  `KANDEV_TEST_POSTGRES_DSN` was not configured; the SQLite ownership tests
  passed.
- Review follow-ups preserve Local drafts when a legacy detached row only has a
  stale path, and select the latest active deadline when primary and secondary
  GitHub quota windows overlap. The manual sync lock regression now uses a real
  coordinator-backed PAT client for both listing and file-read blocks.
- The stored-version behavior remains as specified: Repository Save uses the
  stored repository version, and the next sync refreshes it. All nine issue
  findings retain their recorded dispositions above.
- Changed Go lint reported zero issues. The public documentation validator and
  whitespace check passed.

## Risks

- A request can consume quota during the listing or a file download. Check
  admission for each request and abort before applying any fetched files.
- A detached row can predate the fix. Cover its HTTP and MCP reads while
  its old path is still in storage.
- Reconciliation keeps file and local edit ownership separate. Verify a
  current-path update leaves the user's local text intact.

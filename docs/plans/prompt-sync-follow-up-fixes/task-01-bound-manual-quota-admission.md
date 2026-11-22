---
id: "01-bound-manual-quota-admission"
title: "Bound manual GitHub quota admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-002
  - REQ-INTEGRATIONS-PROMPT-SYNC-005
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-001.6
  - AC-INTEGRATIONS-PROMPT-SYNC-002.5
  - AC-INTEGRATIONS-PROMPT-SYNC-005.2
  - AC-INTEGRATIONS-PROMPT-SYNC-005.4
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 01: Bound manual GitHub quota admission

## Summary

When local GitHub quota blocks a manual Prompt Sync run, return a
retryable error and release the service lock. This covers **Sync now**
and the first sync after configuration Save.

## In scope

- In GitHub `RateAdmission.waitForProviderWindow`, honor the existing
  `WithNonBlockingGitHubAdmission` option for interactive work. When the
  caller can proceed, use normal interactive admission. When quota blocks the
  call, return `AdmissionWaitError` with its provider retry details and an
  explicit cause for the opt-in refusal. Preserve parent cancellation.
- Set the option in `syncLocked` for the manual GitHub run. Reuse the
  request context and the current authorization and apply path. The source
  check reads GitHub outside the service lock.
- Reuse `github.RateLimitErrorCode` for the quota block in the sync
  response and saved status. The existing backoff uses the retry boundary
  from `AdmissionWaitError`. Return a fixed, safe fallback message to API
  callers.
- Translate the rate-limit copy once for sync responses in the existing
  web API client. Translate the saved error in the Prompt Sync status
  card. The current toast handles the case where configuration Save
  succeeded and the sync failed.
- Add retry guidance to the
  [Prompt Sync guide](../../public/prompt-sync.md) in the implementation PR.

### Provider paths

| Provider | Repository read path | Expected behavior and evidence |
| --- | --- | --- |
| GitHub token or App | PAT/Token client REST contents | An opted-in request returns typed retry details before transport when blocked. Test the coordinator and local HTTP transport. |
| GitHub CLI | Legacy resolved `gh api` contents | An opted-in request returns the same typed details before starting `gh`. Use the existing fake CLI fixture. |
| GitLab | GitLab repository tree and file reads | Use the provider's existing admission and error path. Run the Prompt Sync GitLab service tests. |

Automatic sync progress and visibility are item 6 in the
[issue findings](plan.md#other-issue-findings).

## Acceptance

- A known GitHub quota window blocks the list or a later file before apply.
  A waiting version Save can complete while the quota window remains active.
- The API distinguishes a retryable quota block from cancellation and
  other provider failures. The retry deadline honors the provider window.
  A later successful **Sync now** clears the failed status.
- The existing toast and status card show localized retry guidance on
  desktop and phone.

## ASCII UI preview

UI-01 from the [plan](plan.md#ui-01-quota-blocked-manual-sync-in-settings--prompts)
applies to AC-INTEGRATIONS-PROMPT-SYNC-001.6.

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

Use the existing responsive status card. On phone, keep the guidance
visible without hover and use a touch-sized **Sync now** action.

## Verification

Add `TestRateCoordinatorOptInInteractiveAdmissionReturnsQuotaRetry` first.
Set a future quota boundary on the tracker and use a guard deadline in the
test. The original coordinator waits for the guard context and returns
cancellation. The fixed one returns the typed refusal before transport.
Cover both primary quota and a secondary throttle.
Also cover overlapping windows in both directions: an exhausted primary reset
one hour away beats a secondary throttle one minute away, while a healthy
primary snapshot with the same reset does not override a one-minute
secondary-only retry.

Add `TestPromptSyncManualQuotaDoesNotBlockVersionSave` with a concurrent
version save and a blocked manual sync. Table-drive blocks before the
listing and after at least one file read. Assert that fetched files never
reach apply on a blocked run. Compare the next-attempt time to the provider
retry time. Test the shared sync response for **Sync now** and the first
sync after configuration Save.

Use a one-time quota response in the browser tests and let the retry reach
the fixture backend. Use the title phrase `reports a GitHub quota block`
in both projects. The Go fixture owns the quota and lock assertions.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/github ./internal/promptsync)
(cd apps/web && pnpm exec vitest run components/settings/prompt-sync-section.test.tsx lib/api/domains/prompt-sync-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/settings/prompt-sync.spec.ts -- --grep 'reports a GitHub quota block' --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-prompt-sync.spec.ts -- --grep 'reports a GitHub quota block' --retries=0)
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/github/rate_coordinator.go`
- `apps/backend/internal/github/rate_coordinator_test.go`
- `apps/backend/internal/github/repo_contents_test.go`
- `apps/backend/internal/promptsync/service.go`
- `apps/backend/internal/promptsync/failure.go`
- `apps/backend/internal/promptsync/handlers.go`
- `apps/backend/internal/promptsync/handlers_test.go`
- `apps/backend/internal/promptsync/admission_deferral_test.go`
- `apps/web/lib/types/prompt-sync.ts`
- `apps/web/lib/api/domains/prompt-sync-api.ts`
- `apps/web/lib/api/domains/prompt-sync-api.test.ts`
- `apps/web/components/settings/prompt-sync-section.tsx`
- `apps/web/components/settings/prompt-sync-section.test.tsx`
- `apps/web/src/locales/en/settings.json` and the six other shipped catalogs
- `apps/web/e2e/tests/settings/prompt-sync.spec.ts`
- `apps/web/e2e/tests/settings/mobile-prompt-sync.spec.ts`
- `docs/public/prompt-sync.md`

## Dependencies

None.

## Risks

- The GitHub CLI error classifier can replace a typed error with an HTTP
  error. Verify that a quota refusal retains its type in both transports.
- The coordinator admits requests individually. A successful listing does
  not guarantee quota for the next file.
- Preserve parent cancellation when the request ends during admission.

## Parallelism

Sequential. Both work orders touch the same Settings E2E files and guide.

## Inputs

- [Prompt Sync requirements](../../specs/integrations/requirements/prompt-sync.md)
- [Prompt Sync control flow](../../specs/integrations/system-design/prompt-sync.md#control-flow)
- [GitHub provider admission](../../specs/integrations/system-design/github-rate-limit-coordination.md#principal-wide-coordinator)
- `TestRateCoordinatorInteractiveAdmissionCancellationPreservesRateDetails`
- `TestAutomaticRunDeferredByGitHubAdmissionIsNotAFailure`

## Results

Completed.

- Backend quota coordinator and Prompt Sync tests passed, including the manual
  quota response and Save concurrency cases.
- The review regression routes both the blocked listing and the second file
  read through a PAT client wired to the real coordinator. Seeded quota windows
  and guard deadlines prove that the sync returns early and Save finishes while
  quota remains active.
- `TestRateCoordinatorOptInInteractiveAdmissionUsesLatestActiveQuotaDeadline`
  and `TestRateCoordinatorOptInInteractiveAdmissionUsesSecondaryDeadlineWithHealthyPrimary`
  passed. Together they verify that only active primary limits compete with a
  secondary retry deadline.
- Focused Settings and API tests passed. Web typecheck and i18n validation
  passed.
- Desktop Chromium and mobile Chrome browser tests passed for quota guidance
  and retry. The E2E production build completed successfully.
- Public documentation validation and changed Go lint passed.

---
id: "01-order-mutations"
title: "Order Azure saved-view actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.15
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.16
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.17
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.18
system_design:
  - ../../specs/integrations/system-design/azure-devops-integration-02.md
---

# Order Azure saved-view actions

Status: done

## Outcome and scope

Implement the queue and confirmed-baseline recovery in the saved-view hook,
use functional appends in the direct page consumer, handle delete failure, and
prove the actual page menu/confirmation flow on desktop and phone.

## Traceability

- REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.
- AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.15, .16, .17, .18.
- [System design](../../specs/integrations/system-design/azure-devops-integration-02.md#saved-view-mutation-settlement).
- Dependency: none. Ownership: hook, its tests, direct page consumer, targeted
  integration E2E/helper files, owning docs. No unrelated integration edits.

## Acceptance

1. Requests preserve admitted action order and visible intent during pending
   writes, including normalized success and same-tick callbacks.
2. Failed operations reject to callers and later requests use the confirmed
   baseline; initial reads cannot erase pending edits. Retry works.
3. The real page handles delete failure with localized feedback and preserves
   desktop/phone confirmation and retry behavior, with persisted-list proof.

## Exact verification

From `apps/web` with Node 24 on PATH:

```bash
pnpm exec vitest run hooks/domains/azure-devops/use-azure-devops-saved-views.test.ts
pnpm exec eslint hooks/domains/azure-devops/use-azure-devops-saved-views.ts hooks/domains/azure-devops/use-azure-devops-saved-views.test.ts app/azure-devops/azure-devops-page-client.tsx e2e/helpers/azure-saved-views.ts e2e/tests/integrations/azure-saved-views.spec.ts e2e/tests/integrations/mobile-azure-saved-views.spec.ts
NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

From repo root with Node 24 on PATH:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

From `apps/web` (managed runner, one shard/worker, sequential):

```bash
GOMAXPROCS=2 GOFLAGS=-p=2 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --project chromium e2e/tests/integrations/azure-saved-views.spec.ts
../../scripts/run-quiet e2e --summary -- pnpm e2e:run --no-build --project mobile-chrome e2e/tests/integrations/mobile-azure-saved-views.spec.ts
```

Public-doc impact: check Azure integration guidance. Preserve its how-to focus;
add only user-visible save/delete error and retry behavior if needed.

## Initial results (historical)

RED: existing controls passed (5), new ordering/read tests failed (7). Overlap
sent two writes while the first was pending; same-tick deletes restored one
view; pending-read mutations sent an empty list and lost unrelated views.
Additional browser RED against the built original page consumer:
- Both desktop cases reached the real confirmation/save flow and failed on
  missing localized delete-failure feedback.
- The queued-save assertion failed on persisted labels: expected
  `[One, Two, New queue]`, received `[Two, New queue]`. This proves the page's
  snapshot append carried a failed deletion into a later successful write.

Historical GREEN before review remediation:
- Focused Vitest: 19 passed, including every first/second success/failure
  combination for saves and deletes, normalized responses, same-tick deletes,
  pending GET publication while PUT is deferred, retry, initial read failure,
  and separate keyed-page lifetimes.
- Scoped ESLint: passed without warnings.
- Desktop browser: 2 passed (10.2s), fresh managed production build,
  `/tmp/kandev-run.e2e.i20SZX2y.log`.
- Phone browser: 2 passed (13.9s), same production build,
  `/tmp/kandev-run.e2e.8sOo1xPS.log`. Uses touch actions and the real bottom
  menu/confirmation. No layout or target-size change was made.
- Both browser flows query actual backend persistence and reload the page.
  Consecutive deletes prove payload order `[[two], [], []]`, where the second
  request fails and the third retries. Mixed delete/save proves confirmed
  baseline recovery at the real page consumer.
- i18n check and ratchet passed. Existing translated failure copy is reused;
  all six translated catalogs and pseudo remain complete, with no new keys.
- Spec catalog and full spec lint passed. Public docs validators: 63 tests
  passed and 54 published pages validated.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=6144 pnpm run
  typecheck`, including the rich-output contract check. The default 4 GB run
  exited 134 with a JavaScript heap-limit failure. The machine crash's cause
  remains unproven.

Publication preflight: added the existing PR documentation gate's structured
plan/work-order frontmatter. Ran the trusted base's `validateCoverage` evaluator
against all eleven changed paths and the linked requirement/design documents:
`status=covered`, `ok=true`, no errors. At that metadata-only publication, product source and test behavior remained
at the parent-validated implementation; review remediation below supersedes
that historical source state.

Public docs updated: Azure integration how-to in `docs/public/integrations.md`
now describes saved-query ordering, error feedback, and retry. Internal
requirements/design and this plan/work order record the bounded behavior.

Limits: no external live Azure incident, no cross-workspace leak claim, no
multi-tab/remount ordering guarantee, and no backend concurrency or schema
change. Writes are serialized, so a later PUT cannot complete ahead of an
earlier one within this mounted page. API failures remain rejected to callers;
the page catches delete failure and includes its detail in the reporting toast.
The interrupted pre-crash build produced no browser evidence; only recovered
managed-run results above count.

## ASCII UI preview

UI-01: Existing Azure saved-view deletion and failure state.

```text
Desktop: [Saved views v] -> [Delete view] -> [Cancel] [Delete view]
Phone:   [Saved views v] -> bottom menu -> inset confirmation
Failure: [Failed to delete saved query] (existing reporting toast)
         [Saved views v] -> restored view available for retry
```

The existing scope menu, confirmation, scrolling, focus return, and phone
44px targets are retained. Only settlement and failure feedback change.
The phone exemplar is the shipped Azure saved-view confirmation in
`mobile-azure-devops.spec.ts`, using shared phone menus/confirmation drawers.
Copy is illustrative English backed by the existing localization key. Browser
proof covers UI-01 and AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.18.

## Review remediation

The first published head completed CI with 64 passed, 25 skipped, 1 neutral,
and no failed or pending checks. Review found that a failed initial GET did
not establish a confirmed baseline, yet a queued PUT could use the fallback
empty list and overwrite existing server views.

RED: three deterministic cases failed because saves/deletes resolved after a
failed GET instead of rejecting. The corrected cases cover save/delete retry
against the recovered baseline and rejection of every operation bound to the
failed read even when a new retry begins between queue settlements.

The hook now owns mutable queue state in a ref, retains each admitted read's
rejection, and starts a shared fresh read when a later action retries. Failed
baseline reads produce no PUT and discard the affected optimistic operations.
The desktop and phone browser flows additionally admit a save while GET is
held, reject GET, assert localized save feedback and unchanged backend data,
then retry successfully while retaining existing queries.

Post-remediation results (same exact commands as the work order):
- Focused Vitest: 21 passed after the final source edit and test grouping.
- Scoped ESLint passed without warnings. Typecheck with the 6 GB Node heap,
  i18n check, and the i18n ratchet passed.
- Desktop production build: three passed (10.3s),
  `/tmp/kandev-run.e2e.HmVe9ctP.log`.
- Phone: three permanent flows plus one disposable screenshot capture passed
  together (18.0s), `/tmp/kandev-run.e2e.krR5zCxL.log`, same product build.
- The new browser case failed against the prior build because the dialog
  closed rather than reporting the rejected read; historical RED log:
  `/tmp/kandev-run.e2e.hKH4Rali.log`.
- Spec catalog and full spec lint passed. Public-doc validator tests: 63
  passed; 54 published pages validated. PR documentation coverage is rechecked
  against the final changed paths before publication.

The PR body is reconciled with 21 hook cases and three permanent browser flows
per viewport. Existing rendered primitives and copy remain unchanged;
publication captures are refreshed from the rebuilt product. No new API or
backend contract is added. Current-head remote verification and merge are
tracked in [PR #4639](https://github.com/kdlbs/kandev/pull/4639).

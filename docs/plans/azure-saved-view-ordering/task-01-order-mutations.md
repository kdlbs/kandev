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

## Results

RED: existing controls passed (5), new ordering/read tests failed (7). Overlap
sent two writes while the first was pending; same-tick deletes restored one
view; pending-read mutations sent an empty list and lost unrelated views.
Additional browser RED against the built original page consumer:
- Both desktop cases reached the real confirmation/save flow and failed on
  missing localized delete-failure feedback.
- The queued-save assertion failed on persisted labels: expected
  `[One, Two, New queue]`, received `[Two, New queue]`. This proves the page's
  snapshot append carried a failed deletion into a later successful write.

GREEN after the final production edit:
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
`status=covered`, `ok=true`, no errors. Product source and test behavior remain
at the parent-validated implementation.

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

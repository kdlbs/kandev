---
id: "01-reconcile-save"
title: "Reconcile acknowledged repository Save progress"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SAVE-001
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SAVE-001.1
  - AC-WORKSPACES-REPOSITORY-SAVE-001.2
  - AC-WORKSPACES-REPOSITORY-SAVE-001.3
  - AC-WORKSPACES-REPOSITORY-SAVE-001.4
system_design:
  - ../../specs/workspaces/system-design/workspace-settings-updates.md
---

# Task 01: Reconcile acknowledged repository Save progress

## Summary

Preserve fulfilled operations across a rejected Save and make retry operate from
accepted identities and values. Use permanent causal RED before production edits.

## In scope

Page save orchestration, draft/baseline reconciliation, in-flight joining, Reset,
real hook/coordinator regressions, disposable backend desktop/phone persistence proof.

## Out of scope

Backend/schema/idempotency/security changes, query cache migration and other settings.

## Acceptance

- Acknowledged creates, updates and deletes survive failures without duplicate resources.
- All siblings settle before rejection; newer drafts remain dirty and Reset uses accepted state.
- Real coordinator exposes error/retry and desktop/phone reload proves unique persistence.

## UI-01: Existing Save recovery

See [combined preview](plan.md#ui-01-existing-save-recovery-desktop-and-phone).

```text
Repository: Build acknowledged; Lint unsaved
Shared action: Couldn't save | Reset | Save changes
```

Existing desktop and phone composition is preserved (AC .2 through .4).

## Verification

```bash
# Requires Node 24 (select it with your version manager).
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run app/settings/workspace/workspace-repositories-save.test.tsx app/settings/workspace/workspace-repositories-client.test.tsx app/settings/workspace/workspace-repositories-dirty.test.ts components/settings/repository-card.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 app/settings/workspace/workspace-repositories-client.tsx app/settings/workspace/workspace-repositories-save.ts app/settings/workspace/workspace-repositories-dirty.ts components/settings/repository-card.tsx app/settings/workspace/workspace-repositories-section.tsx)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --project chromium tests/settings/repository-save-retry.spec.ts -- --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/settings/mobile-repository-save-retry.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- apps/web/app/settings/workspace/workspace-repositories-client.tsx
- apps/web/app/settings/workspace/workspace-repositories-save.ts
- apps/web/app/settings/workspace/workspace-repositories-dirty.ts
- apps/web/app/settings/workspace/workspace-repositories-section.tsx
- apps/web/components/settings/repository-card.tsx
- apps/web/app/settings/workspace/workspace-repositories-save.test.tsx
- apps/web/e2e/tests/settings/*repository-save-retry*
- docs/specs/workspaces/{requirements,system-design}/workspace-settings-updates.md
- docs/public/ (existing repository-settings guide)

## Dependencies

None.

## Risks

Canonical ID promotion must not lose newer edits or release Save before siblings settle.

## Parallelism

`sequential`

## Inputs

Repository lifecycle design, shared manual-save contract, parent causal diagnostic.

## Results

Done locally. Parent diff validation passed and delivery was authorized. Final
current-head CI, semantic review and merge confirmation remain external gates.

All verification commands above passed. Final targeted Vitest: 70 tests across
4 files. Desktop chromium: 2 passed after a fresh managed production build.
Phone mobile-chrome: 2 passed sequentially against those exact artifacts. One
worker and zero retries for each final run. ESLint: zero warnings. Typecheck:
Node 24, 8 GB heap cap (default 4 GB failed with heap exhaustion). Specification
catalog and full spec lint passed; public-doc tests: 63 passed; public-doc tree:
54 pages validated. i18n checks and ratchet passed. Diff whitespace check passed.

Additional regression evidence: page-level overlapping calls join one pending
save; unchanged saved scripts issue no writes; metadata acknowledgements survive
script failure; scripts added while creating a repository acquire its canonical
owner. The latter was independently proved RED against the prior reconciliation
line before restoring GREEN. Permanent initial suite showed 10 causal failures
and 5 passing controls before production changes.

The PR documentation coverage validator (`validateCoverage` from
`.github/scripts/pr-docs.cjs`) passed with the current changed paths and the four
referenced documents (work order, plan, requirement and design); accepted
REQ-WORKSPACES-REPOSITORY-SAVE-001 and all four ACs.

Public guide updated with Retry save and Reset recovery; no product copy changed.
Phone screenshot inspected. No native subagents, persistent tasks or extra
sessions were created. Source changes stayed in the supplied isolated worktree;
E2Es used disposable fixture checkouts and repository records, preserving the
shared seed repository. Initial selector-only E2E timeout and default-heap
exhaustion were corrected at the fixture/environment boundaries, respectively.

## PR review refinement

The first published implementation head completed CI with 67 passed, 22 skipped,
one neutral, zero failed and zero pending checks. Claude and Greptile found no
blocking correctness issues. CodeRabbit's portable Node prerequisite and per-test
phone screenshot suggestions are addressed: screenshots attach directly to each
Playwright test result. The submitted-order invariant is documented at the draft
merge boundary. Optional suggestions to rewrite the discriminant guard or explain
the map's unconditional deletion are acknowledged without changing correct logic.

Review refinement verification: the four targeted Vitest suites passed all 70
tests again. Revised desktop and Pixel 5 flows each passed two tests with one
worker and zero retries. Changed-file ESLint passed. Phone screenshots now attach
to the per-test Playwright report without a shared output filename.

Final-head CI/review and merge confirmation are external delivery gates; the
first-head counts above are historical evidence, not final-head claims.

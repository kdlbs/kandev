---
id: "03-feature-evidence"
title: "Feature evidence"
status: pending
wave: 3
depends_on:
  - "02-coordinator-page"
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-001
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-002
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-003
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-004
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-005
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-006
acceptance_criteria:
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.4
  - AC-ORCHESTRATION-COORDINATOR-VIEW-002.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-002.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-002.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-003.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-003.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-003.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-003.4
  - AC-ORCHESTRATION-COORDINATOR-VIEW-004.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-004.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-004.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-005.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-005.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-006.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-006.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-006.3
system_design:
  - ../../specs/orchestration/system-design/coordinator-view.md
---

# Task 03: Feature evidence

## Summary

Prove the delivered Coordinator flow alongside the existing coordinator and
automation regressions, then document and capture its actual behavior with
synthetic data. Keep the evidence suitable for the focused future PR.

## In scope

- Real UI E2E with disposable fixtures, canonical synthetic task/session states,
  multiple workspaces/coordinators and deterministic delayed responses.
- Desktop/mobile capture of central tasks plus chat, input/task navigation and
  selected-coordinator filtering; short silent video with generic prompts.
- Update the existing orchestration public page as a how-to guide, linked API
  reference if client behavior changes, scope/validation records and PR draft.

## Out of scope

Live data, real user prompts, provider-quality claims, publishing media/comments,
opening a PR, deploying, or claiming unrelated assistant functionality is complete.

## Acceptance

- The new browser flow and both existing orchestration specs pass together with
  no retries, covering the plan's criteria and desktop/mobile behavior.
- Screenshots/video contain only fixture-owned fictional records and generic
  requests; every frame is reviewed and no real prompt/history/credential appears.
- Documentation and the PR draft describe only delivered behavior and actual
  validation, and keep the exact v0.94.0 baseline and remaining release limits clear.

## Verification

From repository root, using the repository's managed E2E launcher:

```bash
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium -- e2e/tests/orchestration/coordinator-view.spec.ts e2e/tests/orchestration/workspace-orchestrators.spec.ts e2e/tests/orchestration/automation-orchestrator.spec.ts --retries=0
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome -- e2e/tests/orchestration/mobile-coordinator-view.spec.ts --retries=0
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the capture/export commands supplied by the current `e2e` skill. Record exact
commands, fixture provenance, build SHA, media hashes and frame-review results
with the artifacts; do not substitute historical prototype media. A safe generic
request is “Summarize the open Garden Notes tasks and suggest the next step.”

## Files likely touched

- `apps/web/e2e/tests/orchestration/coordinator-view.spec.ts` (new).
- `apps/web/e2e/tests/orchestration/mobile-coordinator-view.spec.ts` (new).
- `apps/web/e2e/helpers/orchestration.ts` and fixture-owned capture helpers.
- `docs/public/orchestration-personas.md`; `docs/public/feature-status.md` and
  coverage metadata only where their existing contracts require updates.
- This plan/work-order results and the local contribution review packet.

## Dependencies

Tasks 01 and 02. Fresh backend/frontend builds through the managed fixture are
required; an old running preview is not evidence for the current files.

## Risks

Shared database state previously made the two prototype specs order-sensitive.
Cleanup must delete only fixture-owned registrations/tasks and preserve shared
conversations correctly. Captures must use isolated data and scripted replies.

## Parallelism

`sequential`

## Inputs

- All coordinator-view criteria and the plan's test map.
- Existing coordinator/automation specs, fixture cleanup and the current `e2e`,
  `mobile-parity` and `docs-maintainer` skills.

## Results

Pending. Existing issue media documents the earlier prototype only.

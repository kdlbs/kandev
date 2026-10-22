---
id: "08-operator-policy-learning"
title: "Deliver policy selection and explicit learning"
status: pending
wave: 8
depends_on:
  - "06-approval-channel"
  - "07-mcp-connectivity"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-002
  - REQ-AGENTS-SANDBOX-004
  - REQ-AGENTS-SANDBOX-006
acceptance_criteria:
  - AC-AGENTS-SANDBOX-002.1
  - AC-AGENTS-SANDBOX-002.2
  - AC-AGENTS-SANDBOX-002.3
  - AC-AGENTS-SANDBOX-002.4
  - AC-AGENTS-SANDBOX-002.5
  - AC-AGENTS-SANDBOX-002.6
  - AC-AGENTS-SANDBOX-004.1
  - AC-AGENTS-SANDBOX-004.2
  - AC-AGENTS-SANDBOX-004.3
  - AC-AGENTS-SANDBOX-004.4
  - AC-AGENTS-SANDBOX-004.5
  - AC-AGENTS-SANDBOX-006.1
  - AC-AGENTS-SANDBOX-006.2
  - AC-AGENTS-SANDBOX-006.3
  - AC-AGENTS-SANDBOX-006.4
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
---

# Task 08: Deliver policy selection and explicit learning

## Summary

Expose effective selections and bounded denial reports, then publish operator-approved immutable revisions.

## In scope

Implement settings/task/workflow selectors, source provenance, report completeness, diff review, suppressions, revision persistence, and responsive UI.

## Out of scope

No automatic learning, live repo-profile mutation, retroactive changes to running attempts, or hidden desktop-only actions.

## Acceptance

- All selection surfaces show source and ceiling and reject unauthorized weakening after reload.
- Learning shows exact access changes and provenance, rejects stale/out-of-ceiling proposals, and changes future selection only explicitly.
- Desktop and phone support loading, empty, error, read-only, approval, denial, and learning flows with all locale keys.

## UI preview

UI-01: Selection and UI-03: Learning. See the [combined preview](plan.md#ui-preview).
Desktop: settings selectors plus provenance, then denial list and revision diff. Phone: bottom picker for selection and full-height review for learning.
Preserve loading, error, expired, and read-only states. Use shared domain state and localized labels.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/sandbox/... -run 'SandboxReport|SandboxLearning|SandboxPolicy' -count=1)
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/sandbox-policies.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-sandbox-policies.spec.ts -- --retries=0
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `apps/backend/internal/agent/sandbox/report_test.go (new)`
- `apps/backend/internal/agent/sandbox/learning_test.go (new)`
- `apps/web/components/settings/profile-edit/`
- `apps/web/components/settings/executor-profile-dialog.tsx`
- `apps/web/components/task/`
- `apps/web/components/workflow/`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/sandbox-policies.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-sandbox-policies.spec.ts (new)`
- `Dedicated Nono plugin: report normalization and proposal compilation`

## Dependencies

06-approval-channel, 07-mcp-connectivity

## Risks

Missing audit data must remain unknown. Suppression must never translate into access.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).

## Results

Pending. No production implementation is authorized during the design turn.

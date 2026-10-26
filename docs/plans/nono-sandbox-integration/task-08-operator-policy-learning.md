---
id: "08-operator-policy-learning"
title: "Deliver denial review and explicit learning"
status: done
wave: 9
depends_on:
  - "06-approval-channel"
  - "07-mcp-connectivity"
  - "10-execution-surfaces"
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

# Task 08: Deliver denial review and explicit learning

## Summary

Extend the shared Execution details surface with bounded denial reports and operator-approved immutable revisions.

## In scope

Implement report completeness, provenance, access-diff review, suppressions, revision persistence, and responsive learning views.
Reuse Task 10 selectors for the explicit learned-revision selection. Keep publication separate from selection and existing attempts.

## Out of scope

No automatic learning, live repo-profile mutation, retroactive changes to running attempts, or hidden desktop-only actions.

## Acceptance

- Report and learning views preserve source and ceiling information. Published revisions do not change selection without an explicit authorized action.
- Learning shows exact access changes and provenance, rejects stale/out-of-ceiling proposals, and changes future selection only explicitly.
- Desktop and phone support loading, empty, error, read-only, approval, denial, and learning flows with all locale keys.

## UI preview

UI-03: Learning extends UI-05: Execution details. See the [combined preview](plan.md#ui-preview).
Desktop: open the denial list from Execution details, then review the access diff and persistence scope.
Phone: navigate within the full-height detail surface to the diff, with fixed Back and Create revision controls.
Use Task 10 policy pickers only after explicit selection of the new revision. Map to SANDBOX-004.1-5 and SANDBOX-006.1-2.
Preserve loading, error, expired, and read-only states. Use shared domain state and localized labels.
Denial visibility stays inline in the same scroll body: record selection, Hide selected denials, Show hidden denials, and a hidden-record count. Use 28px desktop controls and at least 44px phone/coarse-pointer targets. Hiding moves focus to Show hidden denials. Restoring moves focus to the first restored record. Hidden entries match kind, operation, and target only within the current attempt view. Refresh preserves hiding; closing the view clears it. Hiding clears the record selection. No visibility action sends a backend write or changes policy selection.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/sandbox/... -run 'SandboxReport|SandboxLearning|SandboxPolicy' -count=1)
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web exec vitest run hooks/domains/session/use-sandbox-denial-visibility.test.tsx hooks/domains/session/use-sandbox-learning.test.tsx hooks/domains/session/use-sandbox-report.test.tsx
pnpm --dir apps/web e2e:run --host --project chromium tests/task/sandbox-reports.spec.ts tests/task/sandbox-learning.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sandbox-reports.spec.ts tests/task/mobile-sandbox-learning.spec.ts -- --retries=0
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `apps/backend/internal/agent/sandbox/report_test.go (new)`
- `apps/backend/internal/agent/sandbox/learning_test.go (new)`
- `apps/web/components/settings/profile-edit/`
- `apps/web/components/settings/executor-profile-dialog.tsx`
- `apps/web/components/task/`
- `apps/web/components/settings/workflow-step-agent-profile-selector.tsx`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/sandbox-policies.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-sandbox-policies.spec.ts (new)`
- `Dedicated Nono plugin: report normalization and proposal compilation`

## Dependencies

06-approval-channel, 07-mcp-connectivity, 10-execution-surfaces

## Risks

Missing audit data must remain unknown. Suppression must never translate into access.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).

## Results

Implementation is authorized. Bounded report storage and trusted runtime capture are implemented. SQLite and PostgreSQL report persistence checks pass. The actual packaged Nono runtime retains denied reads, preserves prior evidence when audit parsing fails, and captures the final report before artifact cleanup. Session-scoped HTTP reads recheck authorization after the runtime read and sanitize failures. Seven frontend tests cover attempt binding, failed refresh, disconnect, and account changes. Desktop and phone report rendering checks pass, including failed refresh, final captures, empty captures, long paths, focus return, and 393/700/767px phone widths. Both screenshots were inspected. Typecheck, scoped lint, localization, public-doc validation, SQL guard, and spec validation pass. Learning proposals and explicit publication are implemented. SQLite and PostgreSQL race tests verify one publication winner, unchanged selections and active attempts, stale evidence rejection, and provenance retention after attempt deletion. Actual Nono proposal compilation passes through the packaged provider. The learning review UI and localization are implemented. Desktop and phone publication checks pass, including explicit diff review, no automatic selection, focus return after publication, and 393/700/767px phone geometry. All seven locales pass the copy checks. Both screenshots were inspected. Presentation-only suppression is complete.

The report extends the existing Execution details body. The phone surface keeps its fixed Back control and one scroll owner. Report refresh and source disclosure use 44px phone/coarse-pointer targets. Desktop controls retain the surrounding density. Empty and incomplete captures never imply that no operations were denied.


Presentation-only visibility checks passed on 2026-10-26. The seven visibility cases cover repeated notices, scope changes, provider changes, and unchanged evidence. The report/learning/visibility unit set passes 18 tests. Desktop and phone report and learning scenarios pass, including focus after hide/restore, no backend write, close/reopen reset, and 393/700/767px phone geometry. All four fresh screenshots were inspected. Typecheck, scoped lint, all seven locales, public docs, and specification checks pass. These fixture-based browser results do not establish actual managed-agent acceptance.

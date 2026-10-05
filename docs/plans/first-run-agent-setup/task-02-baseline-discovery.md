---
id: "02-baseline-discovery"
title: "Share baseline profile discovery"
status: complete
wave: 2
depends_on:
  - "01-opencode-arguments"
plan: "plan.md"
requirements:
  - REQ-AGENTS-FIRST-RUN-SETUP-002
  - REQ-AGENTS-FIRST-RUN-SETUP-003
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
  - REQ-AGENTS-PROFILE-DISCOVERY-003
acceptance_criteria:
  - AC-AGENTS-FIRST-RUN-SETUP-002.1
  - AC-AGENTS-FIRST-RUN-SETUP-002.4
  - AC-AGENTS-FIRST-RUN-SETUP-002.5
  - AC-AGENTS-FIRST-RUN-SETUP-002.6
  - AC-AGENTS-FIRST-RUN-SETUP-003.5
  - AC-AGENTS-PROFILE-DISCOVERY-001.1
  - AC-AGENTS-PROFILE-DISCOVERY-002.1
  - AC-AGENTS-PROFILE-DISCOVERY-002.3
  - AC-AGENTS-PROFILE-DISCOVERY-003.1
  - AC-AGENTS-PROFILE-DISCOVERY-003.2
  - AC-AGENTS-PROFILE-DISCOVERY-003.3
system_design:
  - ../../specs/agents/system-design/first-run-agent-setup.md
  - ../../specs/agents/system-design/profile-capability-discovery.md
---

# Task 02: Share baseline profile discovery

## Summary

Expose the existing saved/draft baseline discovery as one domain hook shared by
quick setup and full profile capabilities. A baseline-only consumer must not
resolve or reconcile dependent model options; the full form still does both.

## In scope

- Extract baseline discovery and tightly coupled context helpers from
  `use-profile-model-capabilities.ts` into proposed
  `use-profile-capability-discovery.ts`, keeping the existing API boundary.
- TDD new hook tests for saved auto-probe, saved environment/flags/prefix context,
  no dependent-option call, explicit refresh, static providers, failed/empty
  responses, profile switch, and late-response rejection.
- Compose the extracted hook from the full capabilities hook and preserve
  existing dependent resolution, reconciliation, draft staleness, and refresh.
- Keep launch settings component-local and preserve secret/redaction boundaries.

## Out of scope

- Tour markup and mutation implementation (Task 03).
- New discovery endpoints, server caches, request semantics, runtime flags,
  schema changes, or full-editor presentation changes.

## Acceptance

- Saved concrete profiles automatically discover matching baseline models;
  baseline-only calls never invoke `resolveAgentModelConfig` or mutate draft options.
- Existing profile identity, context matching, sequence guards, explicit draft
  refresh, failures, and static-provider behavior remain intact.
- The full capabilities hook still resolves/reconciles model options and passes
  the existing form and hook regressions with no rendered layout change.

## Verification

Workspace dependencies are already installed. For a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)` once. Then run from repo root:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/settings/use-profile-capability-discovery.test.tsx hooks/domains/settings/use-profile-model-capabilities.test.tsx components/settings/profile-form-fields.test.tsx)
(cd apps/web && pnpm exec eslint hooks/domains/settings/use-profile-capability-discovery.ts hooks/domains/settings/use-profile-model-capabilities.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/settings/use-profile-capability-discovery.ts` (new)
- `apps/web/hooks/domains/settings/use-profile-capability-discovery.test.tsx` (new)
- `apps/web/hooks/domains/settings/use-profile-model-capabilities.ts`
- `apps/web/hooks/domains/settings/use-profile-model-capabilities.test.tsx`
- `apps/web/hooks/domains/settings/use-profile-model-options.ts` only if needed
  for the extracted baseline type import; preserve its logic.

## Dependencies

Task 01 precedes this in the sequential package. The hook extraction itself has
no runtime dependency on the OpenCode argument correction.

## Risks

Changed callback/reference identities can retrigger probes or stale option
resolution. Use deferred response tests and rerun the existing full-form suites.
Do not promote an agent-wide snapshot into a profile context during extraction.

## Parallelism

`sequential`

## Inputs

- [Shared baseline discovery](../../specs/agents/system-design/first-run-agent-setup.md#shared-baseline-discovery)
- [Existing profile discovery design](../../specs/agents/system-design/profile-capability-discovery.md)
- Existing `use-profile-model-capabilities.test.tsx` and `useProfileModelOptions`.

## Results

PR integration preserved the landed runtime-update contract: the full editor
refreshes its current draft after a matching successful update, retains matching
catalog/runtime information during refresh or failure, and keeps runtime details
out of model settings. The extracted baseline waits for advertised dynamic
support; a red/green hydration regression covers omitted support metadata.
The merged seven-file Vitest run passed 72 tests. Managed browser verification
passed 5 Chromium and 9 mobile-chrome tests, including second-tab runtime updates.
The failed-update E2E compares probes with its pre-update hydration baseline,
proving the failed update adds no discovery request and preserves its catalog.

Extracted saved-profile baseline discovery without introducing model-option calls
into onboarding. Automatic probing, context isolation, refresh, and late-response
guards pass in the focused discovery and full-profile hook tests. The combined
frontend regression run passed all 67 tests on 2026-10-05.

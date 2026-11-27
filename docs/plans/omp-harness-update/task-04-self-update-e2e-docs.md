---
id: "04-self-update-e2e-docs"
title: "Verify and document OMP self-updates"
status: done
wave: 4
depends_on:
  - "03-self-update-settings-ui"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-003.1
  - AC-AGENTS-RUNTIME-UPDATES-003.2
  - AC-AGENTS-RUNTIME-UPDATES-003.3
  - AC-AGENTS-RUNTIME-UPDATES-003.5
  - AC-AGENTS-RUNTIME-UPDATES-003.12
  - AC-AGENTS-RUNTIME-UPDATES-003.14
  - AC-AGENTS-RUNTIME-UPDATES-003.15
  - AC-AGENTS-RUNTIME-UPDATES-003.16
system_design:
  - ../../specs/agents/system-design/harness-self-update.md
---

# Task 04: Verify and document OMP self-updates

## Summary

Prove the operator-facing update flow in desktop and phone browser projects using the repository's intercepted API fixture pattern. Update public agent documentation to distinguish OMP's harness-owned self-update from Kandev's pinned managed npm runtimes.

## In scope

- Add Playwright coverage for OMP status, dialog/drawer without version selection, target-free approval, streamed output/result, and a failed-job UI state. Use fixture data where the post-update ACP-reported version differs from the stable latest reference; assert that both versions are shown with the reference-only/configured-channel explanation. Also intercept a pre-enqueue `up_to_date` response with empty `job_id`; assert the local terminal result appears before the status refresh completes and remains visible when that refresh fails, without job polling.
- Keep desktop and mobile E2E flows in separate spec files so each Playwright project discovers its intended tests.
- Verify the phone drawer remains bounded and its primary action is touch-reachable.
- Document OMP's `omp update` Settings action, package-manager detection, lack of version selection/rollback, configured-channel behavior, and that stable latest is a reference which may differ from the installed result.
- Keep E2E fixture types synchronized with the new update mode.

## Out of scope

- Running a real OMP self-update in CI or altering a developer's installed OMP binary.
- Docker/SSH/Sprites installer changes or package-manager integration tests against real user installations.

## Acceptance

- Chromium E2E proves status, self-update dialog, no version picker, approval with an exact `{}` request body, streamed result, and display of an actual version that differs from the stable reference; it renders a failed job result without implying backend capability publication. An empty-ID `up_to_date` response renders locally before a deliberately failed status refresh, remains visible, and triggers no job request.
- `mobile-chrome` E2E proves the drawer explains the reference/configured-channel distinction, sends the same target-free approval body, fits its bounded scroll area, and keeps the action reachable; an empty-ID `up_to_date` response renders locally and refreshes status without job polling.
- Public docs state that the Settings action runs OMP's own package-manager-aware updater on the Kandev host, follows OMP's configured channel, and does not support version pinning or rollback.

## Verification

```bash
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
make -C apps/backend e2e-plugin-ui
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/settings/agent-self-update.spec.ts)
(cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/settings/mobile-agent-self-update.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/settings/agent-self-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-self-update.spec.ts`
- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`
- `docs/public/agents-and-profiles.md`
- `docs/public/add-agent-cli.md`

## Dependencies

Task 03 provides typed UI handling and the self-update dialog variant.

## Risks

- The E2E fixture must show stable reference metadata and a different actual post-update ACP version without starting a real self-update process.
- The public wording must not imply Kandev stages, rolls back, or changes OMP's configured channel.

## Parallelism

`sequential`

## Inputs

- Requirement AC-AGENTS-RUNTIME-UPDATES-003.1-.5, .12, and .14-.16.
- [Harness self-update design](../../specs/agents/system-design/harness-self-update.md), Control flow and Failure and recovery.
- Existing `agent-runtime-update.spec.ts` and `agent-runtime-update-helpers.ts` route-interception patterns.
- [E2E runner guidance](../../../apps/web/e2e/README.md).

## Results

Desktop and phone Playwright flows passed (3 Chromium and 2 mobile-chrome cases) against the built backend and web E2E runtime. Captured and visually reviewed the reference-only dialog and bounded phone drawer; confirmed the touch action remains reachable after the transient system-update toast clears. The exact `{}` approval body, ACP-reported 18.3.4 result versus stable reference 18.3.2, streamed output, failed-job result, and pre-enqueue no-job result with a failed status refresh/no polling are covered. Existing pinned-update desktop and mobile representative flows passed.

Public docs describe host-only `omp update`, package-manager detection, configured channels, reference-only stable latest, and no Kandev version pin or rollback. Public-doc validators passed (62 tests and 47 published pages). CI fixtures intentionally do not alter an installed OMP executable.

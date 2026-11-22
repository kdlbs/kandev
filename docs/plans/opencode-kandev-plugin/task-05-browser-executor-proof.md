---
id: "05-browser-executor-proof"
title: "Prove shell outcomes across browsers and owning executors"
status: in_progress
wave: 5
depends_on:
  - "04-automatic-launch"
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-PLUGIN-001
  - REQ-AGENTS-OPENCODE-PLUGIN-003
  - REQ-AGENTS-OPENCODE-PLUGIN-004
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-009
acceptance_criteria:
  - AC-AGENTS-OPENCODE-PLUGIN-001.1
  - AC-AGENTS-OPENCODE-PLUGIN-001.2
  - AC-AGENTS-OPENCODE-PLUGIN-001.3
  - AC-AGENTS-OPENCODE-PLUGIN-003.3
  - AC-AGENTS-OPENCODE-PLUGIN-003.4
  - AC-AGENTS-OPENCODE-PLUGIN-003.5
  - AC-AGENTS-OPENCODE-PLUGIN-003.6
  - AC-AGENTS-OPENCODE-PLUGIN-003.7
  - AC-AGENTS-OPENCODE-PLUGIN-004.2
  - AC-AGENTS-BACKGROUND-WORK-007.6
  - AC-AGENTS-BACKGROUND-WORK-007.7
  - AC-AGENTS-BACKGROUND-WORK-007.8
  - AC-AGENTS-BACKGROUND-WORK-007.9
  - AC-AGENTS-BACKGROUND-WORK-009.4
  - AC-AGENTS-BACKGROUND-WORK-009.5
  - AC-AGENTS-BACKGROUND-WORK-009.6
system_design:
  - ../../specs/agents/system-design/opencode-kandev-plugin.md
  - ../../specs/agents/system-design/background-work-opencode.md
---

# Task 05: Prove shell outcomes across browsers and owning executors

## Summary

Prove the complete launch-to-viewer result on desktop, phone, and remote
executors. Preserve legacy tests and separate deterministic mock coverage from
the real packaged-provider path.

## Scope and owned files

- Own bridge-enabled scenarios in
  `apps/web/e2e/tests/task/opencode-background-work.spec.ts` and
  `apps/web/e2e/tests/task/mobile-opencode-background-work.spec.ts`.
- Own a proposed isolated packaged-provider helper
  `apps/web/e2e/helpers/opencode-plugin-runtime.ts`, its configuration, and
  gated packaged smoke cases beside those suites.
- Own new `apps/web/e2e/tests/ssh/opencode-plugin.spec.ts`,
  `apps/web/e2e/tests/docker/opencode-plugin.spec.ts`, and
  `apps/web/e2e/tests/kubernetes/opencode-plugin.spec.ts`.
- Own only the mock-agent and test fixture changes needed for native callback
  events. New enabled assertions must enter the real plugin receiver and journal.
- Update `protocol-evidence.md` with exact fixture, package, and executor results.

Do not replace the current flag-off scenarios. They prove the unknown fallback
and unsupported controls. Do not post directly to background-work persistence
to demonstrate plugin delivery.

The packaged smoke uses the selected real OpenCode 2.2.0 CLI, shipped plugin,
normal launch composition, callback listener, journal, backend, and browser.
A private local model stub supplies deterministic responses. Keep HOME, XDG,
configuration, workspace, and all state disposable. Do not copy user auth or
load user plugins. Record the CLI and asset hashes.

Cover success, actual nonzero exit, early completion, post-prompt completion,
successor-prompt ownership, duplicate delivery, browser refresh, and plugin loss.
Assert no extra root completion, message, task, workflow move, or usage effect.

Remote proof verifies automatic artifact staging and a loopback-only callback
inside the owning executor. Backend-visible host PIDs and paths are not proof.
Use the existing container project for every real Docker, SSH, and Kind case.

## ASCII UI preview

Use [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview). Reuse existing
components and copy. No output or process control is added.

```text
UI-01 desktop: [History] -> Tests [Completed, exit 0] -> named detail tab
UI-02 phone:   [History] -> job drawer -> [Back] Tests [Completed, exit 0]
```

The phone detail keeps one vertical scroll owner, safe-area clearance, and
44px touch targets. Preserve the composer draft, Back behavior, and focus return.
Failure shows the actual nonzero exit. Missing evidence keeps Unknown.

## Acceptance

1. Enabled desktop and phone tests receive terminal state through the receiver,
   retain history after zero active jobs, preserve drafts, and hide controls.
2. A packaged CLI smoke proves actual automatic plugin loading and post-prompt
   settlement. Timing and duplicate cases preserve the original conversation.
3. Docker, SSH, and Kubernetes tests prove executor-local staging and delivery.
   Disabled/incompatible cases preserve ordinary ACP and legacy unknown state.

## Verification

Use TDD for new fixture logic. Add focused helper tests. For state-only browser
assertions, record the lower-level failing regression that supplied the RED gate.
Use event barriers and observable conditions, never unsanctioned sleeps.

From `apps/`:

```bash
pnpm --filter @kandev/opencode-plugin test:conformance
```

From `apps/web/`, run sequentially:

```bash
pnpm e2e:run --host --project chromium e2e/tests/task/opencode-background-work.spec.ts
pnpm e2e:run --host --project mobile-chrome e2e/tests/task/mobile-opencode-background-work.spec.ts
KANDEV_E2E_REAL_OPENCODE=1 pnpm e2e:run --host --project chromium e2e/tests/task/opencode-background-work.spec.ts -- --grep 'packaged OpenCode plugin'
KANDEV_E2E_REAL_OPENCODE=1 pnpm e2e:run --host --project mobile-chrome e2e/tests/task/mobile-opencode-background-work.spec.ts -- --grep 'packaged OpenCode plugin'
KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers -- --grep 'OpenCode plugin executor'
```

Introduce `KANDEV_E2E_REAL_OPENCODE` only in the test harness. The packaged cases
must set and verify both runtime flags explicitly. The container grep must
select nonzero tests for Docker, SSH, and Kubernetes. Use `--list` to prove
selection before execution. Keep the guarded worker and shard budgets.

The managed runner rebuilds the production backend and web assets. Do not test
stale binaries or overlap full suites. Record separate counts for deterministic,
packaged, desktop, phone, and each executor category.

## Exclusions and dependencies

Task 04 must pass. This work does not introduce new viewers, output capture,
native controls, liveness, automatic responses, or historical backfill. Update
the design explicitly before any necessary product-scope change.

## Risks

Mock callbacks cannot prove OpenCode's plugin loading. A running native shell
list omits exited commands. Use the bridge's actual terminal events and committed
receipts. A platform without a proved packaged artifact remains unsupported.

## Results

In progress.

- Added and ran `TestOpenCodePluginPackagedShellOutcome` with
  `KANDEV_OPENCODE_REAL_E2E=1`: real `@opencode/cli@2.2.0`, real embedded
  artifact, real private host, ACP launch claim, callback, journal, and
  `background_work_updated` projection. It records `failed`, exit 3, retained
  source association, and no controls or completion side effects.
- Packaged conformance in the plugin package separately proves automatic plugin
  loading, subscription readiness, additive configuration, a preserved user
  plugin, and one terminal observation.
- Existing desktop and phone background-work suites continue to define the
  flag-off unknown fallback and were not modified. They were not re-run in this
  implementation turn.
- Dedicated bridge-enabled browser scenarios and Docker, SSH, and Kubernetes
  executor runs remain pending. No remote executor or packaged browser result is
  claimed.

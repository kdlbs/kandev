---
id: "03-responsive-e2e"
title: "Responsive E2E"
status: complete
wave: 3
depends_on: ["01-task-disclosure", "02-profile-status-hierarchy"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 03: Responsive E2E

## Acceptance

- Desktop E2E proves a Kubernetes task disclosure is structured, a manual
  refresh stays busy until its exact read settles, updated Pod facts publish,
  and no duplicate read/reset control appears.
- Desktop settings E2E proves direct profile navigation exposes cluster test,
  executor-wide sessions, and Edit cluster connection before profile fields.
- Pixel 5 E2E proves Drawer capability parity, 44 px actions, content-sized YAML,
  one vertical page scroll owner, and no document horizontal overflow.

## Files likely touched

- `apps/web/e2e/tests/settings/kubernetes-task-environment.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts`
- `apps/web/e2e/tests/settings/kubernetes-executor.spec.ts`
- `apps/web/e2e/tests/settings/mobile-kubernetes-executor.spec.ts`
- `docs/plans/kubernetes-executor-ux-polish/task-03-responsive-e2e.md`
- `docs/plans/kubernetes-executor-ux-polish/plan.md`

## Dependencies

Tasks 01 and 02.

## Parallelism

`sequential`. These scenarios validate the integrated UI and must run only
after both production surfaces settle.

## Inputs

- All user-visible scenarios added to the Kubernetes executor spec on
  2026-08-31.
- Existing isolated backend fixture, mobile Pixel 5 project,
  `assertNoDocumentHorizontalOverflow`, and causal request-wait patterns.
- Existing mobile task-environment seeding helper and Kubernetes settings specs;
  no real cluster is required for these frontend behavior checks.

## TDD sequence

1. Add the desktop disclosure spec with a deferred exact status route and
   confirm the current Refresh busy/update assertions fail for the diagnosed
   reason.
2. Extend desktop/mobile profile specs to assert first-section order,
   executor-wide sessions, current-draft diagnostic payload, connection route,
   YAML grow/shrink geometry, touch targets, and overflow.
3. Run fresh built artifacts with retries disabled in both projects. Preserve
   Playwright artifacts for any failure; remove only task-created temporary
   capture files after recording evidence.
4. Record exact discovery/pass counts, per-spec artifacts, backend/frontend
   teardown evidence, and synchronize task/plan status.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web build:e2e && pnpm --filter @kandev/web e2e:raw --project=chromium e2e/tests/settings/kubernetes-task-environment.spec.ts e2e/tests/settings/kubernetes-executor.spec.ts --retries=0 && pnpm --filter @kandev/web e2e:raw --project=mobile-chrome e2e/tests/settings/mobile-kubernetes-task-environment.spec.ts e2e/tests/settings/mobile-kubernetes-executor.spec.ts --retries=0 && cd web && pnpm run e2e:sleep-ratchet && pnpm run typecheck && git diff --check
```

## Output contract

Report initial RED behavior, final desktop/mobile discovery and pass counts,
artifact paths, exact commands, cleanup/teardown evidence, blockers, risks, and
synchronized task/plan status. Confirm the isolated E2E backend used random
ports and that the main Kandev instance on `:9998` was untouched.

## Results

- RED: the first managed desktop refresh run reached the updated restart value
  but could not select it because the summary lacked a stable restart-count
  hook. After adding that semantic hook, the integrated desktop run exposed a
  test-order mistake: the spec dirtied the profile before exercising the
  connection action, so the real navigation guard correctly kept it on the
  profile. The action assertion was moved before the draft edit; production
  navigation behavior did not change. Failure evidence was captured under
  `apps/web/e2e/test-results/settings-kubernetes-task-e-6d1eb-s-the-refreshed-Pod-summary-chromium/`
  and
  `apps/web/e2e/test-results/settings-kubernetes-execut-eefac-ection-and-profile-settings-chromium/`
  before the managed runner replaced the artifacts on the final green runs.
- GREEN desktop:
  `pnpm e2e:run --no-build tests/settings/kubernetes-task-environment.spec.ts tests/settings/kubernetes-executor.spec.ts -- --retries=0`
  discovered and passed 3/3 Chromium tests in 21.6 seconds. It proves the
  structured Pod summary, in-flight refresh join with no duplicate request,
  updated restart fact, no reset action, leading profile cluster section,
  current-draft diagnostic payload, executor-wide sessions, member boundary,
  and canonical connection navigation.
- GREEN mobile:
  `pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-kubernetes-task-environment.spec.ts tests/settings/mobile-kubernetes-executor.spec.ts -- --retries=0`
  discovered and passed 4/4 Pixel 5 tests in 22.4 seconds. It proves Drawer
  parity, 44 px Pod copy/Refresh/settings and profile controls, member-readable
  sessions, YAML grow/shrink, no vertical textarea scrollbar, contained
  horizontal YAML scrolling, and no document overflow.
- `pnpm run build:e2e` completed from the final production sources in 8.10
  seconds. `pnpm run e2e:sleep-ratchet`, focused ESLint, `pnpm run typecheck`,
  and `git diff --check` passed. The final Playwright result is
  `{ "status": "passed", "failedTests": [] }`.
- The managed runners used random isolated worker ports. No current-run
  backend, Vite, Playwright, or worker-port listener remained after completion.
  The five-day-old disposable seed instance on 47646/48646 was deliberately
  preserved, and the main Kandev process listening on `:9998` was never
  stopped, restarted, or reconfigured. No real Kind cluster was provisioned.
- Blockers: none. External side effects: only disposable E2E fixture data,
  cleaned by the managed runner.

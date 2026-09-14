---
id: "08-e2e-and-docs"
title: "Container E2E coverage and operator documentation"
status: pending
wave: 6
depends_on: ["05-launch-non-gating", "07-pre-launch-warning"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-001
  - REQ-EXECUTORS-SSH-REACHABILITY-002
  - REQ-EXECUTORS-SSH-REACHABILITY-003
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-001.9
  - AC-EXECUTORS-SSH-REACHABILITY-001.11
  - AC-EXECUTORS-SSH-REACHABILITY-002.1
  - AC-EXECUTORS-SSH-REACHABILITY-003.1
  - AC-EXECUTORS-SSH-REACHABILITY-003.3
system_design:
  - ../../specs/executors/system-design/ssh-reachability.md
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
---

# Task 08: Container E2E Coverage and Operator Documentation

## Summary

Prove the assembled capability against a real sshd container that is stopped
and restarted mid-test, covering the one thing no unit test can: that both
surfaces flip in the right direction at the right probe. Document the single
new operator configuration key.

## In scope

- `apps/web/e2e/tests/ssh/reachability.spec.ts` under the `containers`
  project, which already gates real-SSH scenarios on `KANDEV_E2E_CONTAINERS=1`
  and owns the `kandev-sshd:e2e` image. Four flows:
  - A reachable host renders the settings panel row as reachable.
  - The sshd container is stopped; after the failure threshold, the panel
    reports the host unreachable by name.
  - The container is restarted; the next probe clears the panel on a single
    success, with no confirming probe.
  - A launch started while the host is stopped is attempted and fails with a
    message naming the host, rather than being refused.
- A short interval set through the documented configuration key for the spec's
  backend, so the threshold is reached in test time rather than in minutes.
- `docs/public/configuration.md` gains
  `executors.sshReachabilityIntervalSeconds` /
  `KANDEV_EXECUTORS_SSHREACHABILITYINTERVALSECONDS`: default, supported range,
  the meaning of `0`, and the clamp behavior.
- `docs/public/executors.md` gains a short SSH reachability section describing
  what the probe answers, what it deliberately does not do (it never gates a
  launch and never repairs a moved host), and where the result appears.

## Out of scope

- Any production code change. If a flow fails, the fix belongs in the work
  order that owns the behavior, not here.
- Generic QA, review, or full-verification sweeps.
- Non-container E2E projects. Every flow here needs a stoppable real host.
- Screenshots or media capture.

## Acceptance

- The stop flow observes the settings panel naming the host as unreachable,
  and does so only after the threshold is reached, not on the first failed
  probe.
- The restart flow observes the panel clearing on a single successful probe.
- Both public documentation pages describe the key and the capability's
  deliberate limits, and the docs build passes.

## Verification

```bash
# From apps/web (requires a running Docker daemon):
KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project=containers tests/ssh/reachability.spec.ts

# Documentation:
pnpm run lint
```

Do not pass all-worker overrides: local runners enforce one worker per shard
and a memory-aware shard budget, and the `containers` project's
worker-scoped backend depends on that serialization.

## Files likely touched

- `apps/web/e2e/tests/ssh/reachability.spec.ts`
- `apps/web/e2e/pages/SSHSettingsPage.ts`
- `apps/web/e2e/helpers/ssh.ts`
- `docs/public/configuration.md`
- `docs/public/executors.md`

## Dependencies

Tasks 05 and 07. The launch flow needs the launch-path attribution and the
pre-launch warning.

## Risks

- These specs cannot run on a host without a Docker daemon, so they are the
  one part of the work package that a contributor may not be able to execute
  locally. Every acceptance criterion in the plan except the four flows here
  has non-E2E evidence, so a blocked run does not blind the rest of the
  package.
- Stopping a container mid-test makes timing part of the assertion. Drive the
  threshold through the configured interval rather than through fixed waits,
  or the spec becomes the slowest and flakiest file in the project.
- `docs/public/**` is website-ready user documentation; a configuration key
  documented with the wrong default or a missing `0` semantic is worse than an
  undocumented one, because an operator will trust it.

## Parallelism

`sequential`

## Inputs

- Plan sections *E2E tests* and *Tests*.
- `apps/web/e2e/README.md` and the `containers` project definition in
  `e2e/playwright.config.ts`.
- `e2e/helpers/ssh.ts` and `e2e/fixtures/ssh-image.ts` for the sshd container
  lifecycle.
- `docs/public/configuration.md` for the existing key-documentation shape.

## Results

Pending.

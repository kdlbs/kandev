---
id: "06-executor-proof-and-docs"
title: "Prove executor flows and document support"
status: in_progress
wave: 6
depends_on:
  - "05-shared-viewer"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-008
  - REQ-AGENTS-BACKGROUND-WORK-009
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-007.1
  - AC-AGENTS-BACKGROUND-WORK-007.2
  - AC-AGENTS-BACKGROUND-WORK-007.3
  - AC-AGENTS-BACKGROUND-WORK-007.4
  - AC-AGENTS-BACKGROUND-WORK-007.5
  - AC-AGENTS-BACKGROUND-WORK-008.1
  - AC-AGENTS-BACKGROUND-WORK-008.2
  - AC-AGENTS-BACKGROUND-WORK-008.3
  - AC-AGENTS-BACKGROUND-WORK-008.4
  - AC-AGENTS-BACKGROUND-WORK-008.5
  - AC-AGENTS-BACKGROUND-WORK-009.1
  - AC-AGENTS-BACKGROUND-WORK-009.2
  - AC-AGENTS-BACKGROUND-WORK-009.3
  - AC-AGENTS-BACKGROUND-WORK-009.4
  - AC-AGENTS-BACKGROUND-WORK-009.5
  - AC-AGENTS-BACKGROUND-WORK-009.6
system_design:
  - ../../specs/agents/system-design/background-work-opencode.md
---

# Task 06: Prove executor flows and document support

## Summary

Prove the complete lifecycle through existing executor boundaries. Record the
supported producer artifact and update user documentation with its real limits.

## In scope

- Extend the deterministic desktop/phone suites with backend/runtime reconnect,
  reset, stale events, active-turn queueing, direct idle send, and retained history.
- Add Docker, SSH, and Kind-backed Kubernetes cases to the containers project.
  Reuse native-codex-app-server-executor.ts patterns and the owning executor
  fixture. Use a deterministic OpenCode-identity producer, not a live user's data.
- Prove no local database, file tail, PID control, or host-network dependency
  bypasses the remote runtime. Feature off starts no new continuity reader.
- Record the Task 02 real packaged CLI result separately from fake-producer
  browser/executor evidence. Record skips and environmental blockers explicitly.
- Update agents-and-profiles.md and tasks-and-workflows.md with the shared chip,
  background activity, Send/Queue distinction, supported runtime, and unavailable
  controls/recovery. Search README.md and docs/screenshots.md for affected copy.
- Reconcile this package, the shared design's provider matrix, and actual
  companion delivery records. Preserve unrelated pending steering statuses.

## Out of scope

A generic QA/review/verification pass, live CI remediation, publication,
flag promotion, paid-model requirements, and unrelated executor changes.

## Acceptance

1. The three-job scenario works through desktop, phone, Docker, SSH, and
   Kubernetes ownership paths with one automatic response and no duplicate turns.
2. Unsupported producer, feature-off, disconnect, and stale-event cases retain
   truthful activity and prompt semantics. Every unavailable environment has an
   explicit result and cannot count as passing evidence.
3. Documentation and protocol evidence match the selected producer and executed
   commands. Prior work-order results are current before default-on promotion.

## ASCII UI preview

No new layout. Verify [OC-UI-01 through OC-UI-03](plan.md#ascii-ui-preview):

~~~text
Desktop: [Background jobs] -> central job tab / overview
Phone:   [Background jobs] -> summary drawer -> full-height detail
Unknown: explicit unavailable activity; no false completion
Terminal: active count clears; history remains reachable
~~~

The same state must produce the same capability and delivery meaning on both
viewports. Reconnect must preserve the draft and return focus predictably.

## Verification

From the Kandev root, after Task 05's dependency install:

~~~bash
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/opencode-background-work.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-opencode-background-work.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --shards 1 --project containers tests/docker/opencode-background-work.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
~~~

Run these managed browser commands sequentially. The proposed docker directory
already matches the containers testMatch. The suite must enumerate
all three executor cases; a missing Docker daemon, SSH fixture, or Kind cluster
is a recorded blocker, not a substituted host-only pass.

Run the exact Task 02 packaged-CLI subprocess command again only if its producer
or consumer wire contract changed. Record the binary digest and prior evidence
otherwise. Run changed-Go lint for files changed by executor integration.

## Files likely touched

- Proposed e2e/tests/docker/opencode-background-work.spec.ts and
  e2e/helpers/opencode-background-work-executor.ts.
- Task 05's desktop/phone suites and owning executor fixtures.
- docs/public/agents-and-profiles.md and docs/public/tasks-and-workflows.md.
- This package's protocol-evidence.md, plan.md, and each completed work order.
- Shared background-work provider matrix and companion records only where
  this implementation changes their previously documented scope.

## Dependencies

Task 05, Task 02's available artifact, and the current containers environment.
Do not mark full continuity complete while producer proof is missing.

## Risks

Docker, SSH, and Kubernetes fixtures require host Docker support. A process ID
or output path valid in one executor is not an address on the backend host.
Browser simulations cannot establish upstream real-model behavior.

## Parallelism

sequential

## Inputs

- [Recovery/security design](../../specs/agents/system-design/background-work-opencode.md#recovery-security-and-observability).
- [Protocol gate](protocol-evidence.md).
- apps/web/e2e/README.md and existing remote executor helpers.
- /docs-maintainer, /e2e, and /mobile-parity.

## Results

Partially implemented on 2026-10-27; the executor and full-continuity evidence
remains blocked on Task 02.

Delivered:

- `docs/public/tasks-and-workflows.md` gained the shared "Inspect background
  work" section: chip, summary, desktop tabs, phone detail, history after the
  active count clears, Send/Queue semantics, unknown activity, unavailable
  output, and observation-only providers.
- `docs/public/agents-and-profiles.md` documents the shared experience and the
  per-provider support boundary, including OpenCode ACP's observation-only
  launches on versions that end their subscription with the foreground prompt.
- The deterministic desktop and phone browser suites exist (Task 05) against the
  real `opencode-acp` adapter identity through `KANDEV_MOCK_PROVIDERS`.

Not delivered (blocked):

- The three-executor containers suite and the one-automatic-response evidence
  require the upstream producer and a Kubernetes/Docker fixture run. No
  substitute host-only pass was recorded.
- The Task 02 packaged-CLI subprocess command was not run; its producer artifact
  does not exist yet.

Public documentation states the limitation explicitly rather than promising
automatic follow-up for OpenCode 2.2.0.

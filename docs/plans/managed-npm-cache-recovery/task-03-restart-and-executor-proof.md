---
id: "03-restart-and-executor-proof"
title: "Prove upgrade recovery and document behavior"
status: done
wave: 3
depends_on:
  - "02-automatic-runtime-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.10
system_design:
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
---

# Task 03: Prove upgrade recovery and document behavior

## Summary

Prove recovery through real restart, session-open, discovery, and executor paths.
Publish the supported behavior and limitations with the implementation.

## In scope

- Deterministic missing-manifest fixture and private preparation on the actual execution host.
- Desktop and phone automatic resume/discovery with no repair or update click.
- Same conversation, accepted prompt once, matching model options, and preserved drafts.
- Preparation/replacement failure, cancellation, sibling safety, and old-peer behavior.
- Local Docker and SSH execution-host proof.
- Public recovery guidance and synchronized delivery records.

## Out of scope

- Public-registry or paid-provider tests, generic UI redesign, missing Codex dependencies, and Antigravity crashes.

## Acceptance

1. Existing affected caches and FAILED sessions recover after backend restart
   through normal page opening. Desktop and phone retain transcript/native ID,
   usable input, and no read-only notice. Discovery succeeds before task creation.
2. Browser tests exercise actual private preparation, final failure, and
   cancellation. Docker/SSH tests prove local preparation and unchanged shared
   sentinels while a healthy sibling serves another request.
3. Documentation states supported recovery and honest failure limits. Results
   record every required command, without marking requirement 005 implemented.

## Presentation and mobile

Reuse the existing task and profile compositions. No markup, copy, navigation,
or control change is planned. Follow the plan's presentation contract and
existing mobile startup-retry/profile-discovery exemplars. Assert existing
touch recovery controls, readable safe details, and no horizontal overflow
on exhaustion. Add a preview before any unexpected rendered UI change.

## Verification

Run commands sequentially from the repository root. The managed E2E runner
builds production assets and backend. Docker and SSH use the containers project.
Record unavailable executor infrastructure as missing acceptance evidence.

```bash
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/managed-npm-cache-recovery.spec.ts tests/settings/managed-npm-cache-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-managed-npm-cache-recovery.spec.ts tests/settings/mobile-managed-npm-cache-recovery.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/docker/managed-runtime-npm-recovery.spec.ts tests/ssh/managed-runtime-npm-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/managed-runtime-startup-retry.spec.ts tests/settings/profile-capability-discovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before completion, run `bash scripts/lint-go-changed` with the exact changed Go
file list from this package. Run the repository documentation-coverage
preflight with all changed contracts and work orders. Record both commands
and results in this work order and the plan.

## Files likely touched

- `apps/web/e2e/helpers/managed-runtime-recovery.ts` and owned npm/registry fixtures.
- New `apps/web/e2e/tests/session/managed-npm-cache-recovery.spec.ts` and mobile counterpart.
- New `apps/web/e2e/tests/settings/managed-npm-cache-recovery.spec.ts` and mobile counterpart.
- `apps/web/e2e/tests/docker/managed-runtime-npm-recovery.spec.ts` and SSH counterpart.
- `docs/public/agents-and-profiles.md` recovery explanation and `docs/public/tasks-and-workflows.md` only where resume guidance changes.
- Requirement/design lifecycle and this plan/work-order results.

## Dependencies

Tasks 01 and 02. Keep old ETARGET/startup scenarios and add the incomplete-cache
matrix beside them. Do not replace the historical scenario coverage.

## Risks

- A fixture that emits only ETARGET does not reproduce the reported defect.
- A browser assertion against seeded status does not prove preparation or restart recovery.
- Container fixture paths and native host paths must identify different caches.

## Parallelism

`sequential`

## Inputs

- Design verification matrix and the plan's E2E mapping.
- `/e2e`, `/mobile-parity`, `/docs-maintainer`, and scoped frontend/backend guidance.
- Existing managed-runtime subprocess, backend restart, and session-open helpers.

## Results

Primary implementation and integration verification completed on 2026-11-07.
All task-defined verification selections are terminal.

- Fresh backend, agentctl (including remote helpers), mock agents, E2E delivery
  fixtures, and plugin artifacts were built with `GOCACHE=/tmp/kandev-go-cache`.
  Browser commands then used `--no-build` against those fresh artifacts.
- Desktop: 2 passed. Phone: 2 passed. The saved-session case starts a real
  managed subprocess, admits one prompt, restarts the backend, retains a FAILED
  row, opens it normally, and preserves the native token and transcript. It
  accepts one follow-up through the native Send control without repair actions.
- Startup and profile discovery use real npm against an isolated local registry.
  Their incomplete shared cache stays incomplete while private preparation
  produces matching capabilities. The profile's saved model remains unchanged.
- Docker and SSH: 4 passed, including both historical temporary-error cases and
  new missing-manifest private preparation on each execution host. Private
  executable files, shared-tree sentinels, and a later prompt are verified.
- `TestManagedRuntimeCacheProcessExitEvidence` verifies real process exit and
  collection. `TestManagedRuntimeCacheNativeCodexProbeCapturesPreInitializeExit`
  verifies the native Codex transport's pre-initialize failure evidence. The
  protocol/process race selection passed after these additions.
- OpenCode v2 command recognition went RED, then GREEN after registering the
  selected distribution alongside v1. The focused three-package rerun passed.
- Preparation failure, replacement failure, cancellation, stale contexts,
  healthy shared-tree preservation, owner lifetime, and old peers are covered
  by Tasks 01/02's backend matrix. Existing browser regressions cover the
  unchanged failure controls and draft/model behavior. These negative cases
  compose backend and existing UI coverage rather than duplicating each in
  another new browser fixture. No production UI changed.
- Public guide updated: `docs/public/agents-and-profiles.md` (how-to guidance).
  Public-doc validator: 55 pages; validator tests: 63 passed. Catalog validation
  and specification lint passed (437 decisions, 1933 specifications). PR docs
  coverage passed for all six changed work orders.
- Go lint passed for every changed package, including the final native probe
  test. Web changed-file lint, formatting, and `git diff --check` passed.
- An exploratory cross-package `-run Profile` selected two existing passthrough
  tests that require a writable global MCP lock. The sandbox rejected their
  setup. The exact task-defined recovery selections and hostutility profile
  selection passed separately.
- Initial browser setup failures concerned fixture PATH, npm UUID redaction,
  lowercase registry precedence, and phone keyboard submission. The final
  cases preserve Node, use reportable cache paths and both registry forms,
  and submit through the Send control. Docker build-state writes use the
  task-owned `BUILDX_CONFIG=/tmp/kandev-cache-recovery-buildx`.

Existing browser regression selection: 7 passed. The primary conversation owns
the normal-hook commit, PR publication, and requested CI/review follow-up.

## PR 5164 remediation

Review reproduced two missing integration cases before the fixes:
`TestManagedRuntimePreparationNativeCodexProbeReplacesFailedProcess` failed
because the native executor passed argv without its executable to the shared
rewriter. `TestPreparedRuntimeResolvesTrustedTemporaryParent` failed when a
trusted temporary parent contained a symlink. Both are now GREEN, including
under the race detector. The private root and descendants retain no-follow
inspection; only the trusted system temporary parent is canonicalized.

CI's Linux, poisoned-environment, and Windows failures shared one existing
stderr expectation. The test now accepts the sanitized path marker and still
explicitly rejects the raw private path. CI lint's repeated Windows literal
uses one package constant. The incomplete-cache ADR now records delivered
recovery while the separate dependency package remains pending.

The desktop and phone session scenarios now exercise both registered OpenCode
families. Their manifest assertions derive the selected package name/version
instead of assuming the v1 package. The fixture stores a complete, validated
family/package selection and restores the original selection after each case.
The expanded desktop and phone selections pass 3 cases each.

The primary conversation records final executor reruns, the normal-hook fixup
commit, exact-head CI results, and review-thread dispositions.

### Additional CI and review remediation

The next CI run passed all backend, frontend, Windows, and container jobs.
Its desktop restart scenario exposed a fixture write during SQLite startup
contention. The write now has a five-second busy timeout and guaranteed
connection cleanup. This changes fixture setup, not application database policy.

New regression tests reproduced the ordinary HTTP timeout, stale flight
registration, configuration-evidence reuse, npm line
boundary, descriptive-path redaction, and utility teardown defects. Tests ran
against the prior production files before the fixes were restored. An actual
owned command also reproduced missing instance-lifetime cancellation.
Publication coverage rejects superseded results. A further regression reproduced
a delayed exit publishing evidence after configuration changed; configuration
now closes the evidence channel and invalidates the snapshot without changing
the launched process generation.

Preparation now uses the long-running client and the caller deadline. Changed
commands or wrappers retire prior startup evidence; unchanged retry configuration
preserves it. A regression covers that required retry handshake. Probe registration and cache
publication share their generation and flight locks. A separate case preserves
returning to a previously valid profile context. The utility handler cancels
and drains active probes before instance cleanup; failed drainage retains the
cleanup hook. Private preparation registers live-consumer guards, bounds process
and output waits, and retains files until consumer cleanup can be confirmed.

Inventory accepts safe leading `./` bin targets and rejects trailing manifest
data. Raw diagnostics discard overlong-line suffixes and redact path-bearing
ENOENT descriptions. Start preparation requires authenticated control. Native
Codex probes leave unverified versions unknown rather than substituting a
compiled schema version. The native process probe asserts both unknown fields.

The npm fixture records JSON argument arrays. Its settings scenario skips
Windows before invoking POSIX fixture tools. Desktop validation uses a temporary
parent containing spaces to exercise argument and path handling.

Focused regression and race command (six affected packages):

```bash
cd apps/backend
GOCACHE=/tmp/kandev-go-cache go test -trimpath -race ./internal/agent/managedruntime ./internal/agent/runtime/agentctl ./internal/agent/hostutility ./internal/agentctl/server/utility ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'TestManagedRuntime|TestPreparedRuntime|TestProfileFlight|TestProfile.*(Context|Refresh|Invalid)|TestRuntimeObservationCodex|TestHandleStart|TestProbeFailureLog|TestProfileProbeFailure' -count=1
```

All six packages pass. Real npm preparation against the local registry passes.
Changed-package Go lint and changed-file web lint pass. Fresh helper builds and
post-remediation browser results are recorded by the primary conversation.
The first repeated desktop run passed five cases and timed out in one v2 resume
case. An isolated v2 rerun passed, then all six repeated cases passed with pprof
enabled for diagnosis. These runs do not establish the timeout's cause. The
later CI capture and targeted readiness regression are recorded below.

Final affected browser selections pass with retries disabled: repeated desktop
6/6 (temporary parent contains spaces), phone 3/3, Docker/SSH 4/4, and existing
startup/profile regressions 7/7. These are 17 distinct cases and 20 passing
executions in the final selections. The earlier isolated timeout remains
recorded above; fresh exact-head CI and review evidence remain externally
pending until the primary conversation completes delivery checks.

## Consolidated PR CI: readiness during automatic recovery

The managed v2 recovery case timed out while the chat was already editable.
Its captured stack was blocked in `isEnabled()` after the recovery action
disappeared between visibility and interaction. The isolated runtime case passed.
A focused browser regression reproduces that same missing-action wait.

The readiness helper now uses normal click actionability within its existing
polling interval. A disappearing or disabled action can time out within that
interval, then readiness is checked again. Other errors still propagate.
The existing idle check and requested editability check remain in place.

The regression failed at the unbounded `isEnabled()` call before the change,
then passed once and in three consecutive runs with retries disabled. The
two actual managed-runtime recovery cases also passed. CI worker context
validation passed 35 cases and skipped one, with retries disabled. Catalog,
specification lint, and delivery coverage for all 11 work orders passed.
Product code and published screenshot surfaces are unchanged.

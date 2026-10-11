---
id: "04-restart-and-oom-acceptance"
title: "Prove restart recovery and validator OOM containment"
status: in_progress
wave: 4
depends_on:
  - "01-restart-safe-cleanup"
  - "02-isolated-validation-runner"
  - "03-guard-validation-entry-points"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001
  - REQ-EXECUTORS-K8S-VALIDATION-001
acceptance_criteria:
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.9
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.10
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.11
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.12
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.13
  - AC-EXECUTORS-K8S-VALIDATION-001.1
  - AC-EXECUTORS-K8S-VALIDATION-001.2
  - AC-EXECUTORS-K8S-VALIDATION-001.3
  - AC-EXECUTORS-K8S-VALIDATION-001.4
  - AC-EXECUTORS-K8S-VALIDATION-001.5
  - AC-EXECUTORS-K8S-VALIDATION-001.6
system_design:
  - ../../specs/executors/system-design/kubernetes-failure-recovery.md
  - ../../specs/executors/system-design/kubernetes-validation-isolation.md
---

# Restart and OOM acceptance

## Summary

Exercise the real lifecycle and cgroup boundary in disposable Kind resources,
including same-conversation continuation and sibling sharing.

## Scope and exclusions

Add focused scenarios using existing Kubernetes/task-Pod/mock-provider fixtures.
Measure independent cgroup ancestry/counters, hard validator limits and exact
Pod/PVC/runtime identities. Extend native conversation observations without
replaying original prompt or making provider network calls. Exclude restarting or
OOM-injecting production Pods and executing the recovered task's PR work here.

## Implementation acceptance

1. Main-container restart followed by cleanup/resume before status polling keeps
   workspace bytes and native conversation ID, starts one successor and delivers
   only the expected queued continuation. Repeat credential retry/backend reload.
2. Two sibling sessions share one validation slot; real lint/build/browser jobs
   complete sequentially. A bounded deliberate child OOM fails only its check,
   while main restart count, control health and conversation identity are stable.
3. Foreign identity, missing accounting/image, over-budget/unbounded workloads,
   cancellation and timeout fail safely; archive removes only exact fixture-owned
   resources and leaves no running validation slot or leaked probe.

## Likely files

Existing: `apps/web/e2e/tests/kubernetes/kubernetes-failure-recovery.spec.ts`,
`kubernetes-task-pod.spec.ts`, `codex-app-server.spec.ts` in that directory;
`apps/web/e2e/fixtures/kubernetes-docker-test-base.ts`,
`apps/web/e2e/fixtures/kubernetes-tools.ts`, `apps/web/e2e/helpers/kubernetes.ts`,
`k8s/worker-images/full/accounting.sh`, `.github/workflows/e2e-tests.yml` and
`.github/workflows/lint-action-pinning.yml`.
Future output: `apps/web/e2e/tests/kubernetes/kubernetes-session-resilience.spec.ts`,
`apps/web/e2e/helpers/kubernetes-validation.ts`,
`apps/web/e2e/fixtures/kubernetes-storage-readiness.ts` and
`apps/web/e2e/scripts/kubernetes-storage-readiness.test.ts`,
`.github/scripts/prepare-full-worker-acceptance.sh` and its Python process tests,
`.github/workflows/kubernetes-session-acceptance.yml` and
`.github/scripts/verify-kubernetes-session-acceptance.cjs`.
Avoid oversized specs by keeping new fault scenarios in that file.

## Scenario controls

Temporarily block status refresh in disposable fixture plumbing, not production
poll constants. Kill only the main container's control process through the
fixture, wait for its observed restart, and invoke normal session.recover.
Use mock-provider request/rollout records to prove native ID/continuation count;
a successful API return alone is insufficient. OOM injection allocates bounded
amount above a small child limit, under a deadline and the companion hard parent.
Do not exhaust the node or the agent container. Fixture-local validation images
must be explicitly streamed into the nested daemon and inspected by exact ID.
Operator digest requirements remain intact. Do not count opt-in skip as pass.

## Verification

Run from repository root on a Docker-capable disposable host. Commands are
sequential. Image build has its existing bounded dedicated builder; capture its
printed IMAGE_ID and set the variable below to that exact ID, not a mutable tag.

```bash
(cd apps && pnpm install --frozen-lockfile)
bash k8s/worker-images/full/build.sh --build --verify
export KANDEV_E2E_FULL_WORKER_IMAGE='sha256:<IMAGE_ID printed by the build>'
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --shards 1 --project containers tests/kubernetes/kubernetes-session-resilience.spec.ts tests/kubernetes/kubernetes-failure-recovery.spec.ts tests/kubernetes/kubernetes-task-pod.spec.ts)
(cd apps/web && pnpm exec eslint e2e/tests/kubernetes/kubernetes-session-resilience.spec.ts e2e/fixtures/kubernetes-docker-test-base.ts)
(cd apps/backend && GOMAXPROCS=2 GOFLAGS=-p=1 go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestKubernetesSharedCleanup|TestKubernetesTaskPod|TestManager_CleanupStaleExecution' -count=1 -timeout=240s)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The image ID placeholder is a required observed runtime input; stop if absent.
Record exact selected/executed/passed test counts and artifact paths, cgroup
limits/ancestry, restart deltas, native IDs and cleanup receipts. Fixture startup
failure is a blocker, not acceptance. Targeted lint supplements test checks;
full lint must not overlap browser execution.

## Dependencies, risks and parallelism

Depends on 01/02/03. Sequential; real Docker image and Kind work have substantial
memory/disk footprints. Existing companion recovery package has historical Kind
setup blockers: report exact new evidence here and reconcile only resolved gates.
Change 2 integration may touch adjacent recovery routes; preserve its ownership.
No production image/profile rollout is authorized in this work order.

## Inputs

[Plan](plan.md), both linked requirements/designs in its frontmatter,
existing Kubernetes fixtures, backend lifecycle tests and resource-safety guidance.

## Results

In progress. New focused scenarios and reusable image-stream/cgroup helpers are
implemented. They assert native ACP conversation identity and prompt uniqueness,
workspace/Pod/PVC retention, bounded shared lint/build/browser checks, real child
OOM, cancellation/timeout cleanup, foreign slot refusal and failed admission.
No fault injection or prevention acceptance has been run in production.

Disposable verification image built as
`sha256:991db2b9f710f1f94650d9a30ca9aa584655dde33f8f424e00918e3a506199e2`,
with enforced BuildKit 4 GiB memory/no extra swap and 2 CPU. The default Docker
bridge is absent on this host. A task-owned labelled network let the same bounded
builder complete; the exact image independently passed the prescribed 4 GiB/2 CPU
non-root tools/source/browser smoke on that network. No image was published.

Initial five-scenario run failed during two fixture image builds and was stopped:
no live assertions executed. The fixture's permission-only image build now uses
`--network=none`. An attempted reuse correctly failed the freshness gate after a
Go import-format change; the next managed run rebuilt backend and web assets.
The focused two-scenario run then hit the original 300-second Kind image-load
limit while exporting a 3.3 GiB archive. Cleanup obscured that original failure.
Only its exact labelled Kind node and archive were reclaimed; the pre-existing
seed cluster was left intact. No containment pass is claimed.

Fixture image load is now bounded at 600 seconds, with corresponding worker
fixture deadlines. Setup cleanup preserves the primary error, attempts both
owned cleanups, and aggregates additional errors. New regression tests were
observed red before that helper existed; 17 targeted fixture-policy, identity,
readiness and kubeconfig tests then passed. Targeted ESLint/formatting passed.
A subsequent focused real run is pending; source/mock tests remain separate from
runtime acceptance. Existing recovery package stays historically blocked until
its real retained scenarios execute successfully.

The next run completed Kind image import within the 600-second operation bound.
The new restart scenario executed live through main restart and retained its
Pod/workspace, then failed because its idle precondition reconnects without a
manual recovery card. Screenshot and DOM snapshot confirmed the normal composer;
this is a test precondition failure, not an accepted recovery gate. The scenario
now holds a traced active turn and disables automatic provider continuation in a
scoped fixture environment before restarting. Initial and interrupted prompts
must each remain unique after native resume. The combined five-scenario run is
selected with one worker, zero retries and one maximum failure during triage.
Fixture-only nested image streaming/readiness deadlines match observed image
transfer size; operator registry-digest readiness remains unchanged.

The combined run selected five tests with retries disabled. Its first retained
recovery scenario failed before recovery assertions: the initial task reached
FAILED because its Pod readiness wait expired (1 failed, 4 did not run). The
fixture removed its exact owned Kind cluster. The list reporter did not preserve
inline diagnostic attachments; a focused two-scenario run now explicitly uses
blob and list reporters to retain Pod inventory, events and logs after cleanup.
No live acceptance pass is inferred from this startup failure.

The diagnostic focused run successfully started the native task Pod/PVC but
failed the test's active-turn precondition (1 failed, 1 did not run). Its native
ACP trace recorded the interrupted prompt and the session was RUNNING, while
`waitForAgentMessage` waited for text persisted only at turn completion. The
scenario now uses the provider trace plus RUNNING state as its causal barrier.
Both focused tests are selected next, with retries disabled and no maximum
failure cutoff, to collect independent runtime evidence. The red run's blob and
sanitized inventory are retained privately under
`/tmp/kandev-session-resilience-evidence/active-precondition-red*`.

Live diagnostics now identify the initial-Pod readiness cause: the local-path
provisioner took 35 seconds to create the PVC, exceeding the unchanged backend
30-second request deadline. Its helper image was already present; task cleanup
removed the expired claim before provisioning completed. Fixture setup now
proves non-root writable storage using a uniquely named bounded Pod/PVC before
seeding task launches, then removes both exact probe resources. It also repeats
readiness after full-worker image import. Two focused readiness/cleanup unit
cases passed; the production request timeout remains unchanged. The obsolete
run was interrupted during its replacement worker image load (exit130): first
scenario failed initial launch, second did not execute acceptance. Its report
and provisioning inventory are retained under the same private evidence folder.

Final local acceptance attempt selected two scenarios with retries0 and
max-failures1, using the repaired fixture. It hit the 600-second Kind image-load
deadline before the storage probe or either test body could execute: 1 fixture
failure, 1 did not run, and an additional cleanup error (exit1). Export alone
took about six minutes on this host. The aggregate error preserves the original
ETIMEDOUT. This is an external live-verification blocker; no native recovery or
child-OOM containment pass is claimed. The exact labelled leftover node was
stopped and removed separately; the unrelated seed cluster was retained. Report:
`/tmp/kandev-session-resilience-evidence/repaired-fixture-image-load-timeout.zip`.
The remaining gate needs a clean Docker-capable runner; new specifications stay
draft and the older companion recovery package remains historically blocked.

Final targeted race verification passed after the last Go format change:
agentctl1.101s and lifecycle9.024s, covering typed control errors, shared cleanup,
task-Pod lifecycle and stale execution cleanup. The command used trimpath,
GOMAXPROCS2 and GOFLAGS-p1. Catalog validation (358 decisions/1414 specs), all
specification lint, actual offline documentation coverage (covered) and diff
whitespace checks passed. New readiness/cleanup cases were included in a16-case
fixture pass; the separate3-case helper suite passed in isolation after a
contention timeout during image loading. Final public docs validation had passed
62 tests and47 pages earlier; that public source is unchanged since the pass.

Targeted Go lint with the pinned image's linter, concurrency2 and a3GiB soft
heap target hit its5-minute bound while loading the repository's fts5/e2e-tagged
package variants. It printed zero findings but exited4 for timeout; this is not
a lint pass. One unchanged targeted rerun uses the now-warmed package caches
under the same bound, without overlapping browser execution.

The unchanged targeted Go-lint rerun passed with zero issues (exit0). CI wiring
now detects the shard that actually selects the resilience spec, builds/verifies
the full worker under its existing4GiB/2CPU policy, and exports only the observed
exact image ID. Failed build/verification or ambiguous image output cannot enable
acceptance. Only that GitHub-hosted shard reclaims unused preinstalled SDKs to
meet the unchanged20GiB disk gate. The build log is retained as an artifact.
Four real process-boundary preparation tests cover shard selection, exact ID,
failed verification and invalid output; they were observed red before the helper
existed. No workflow was dispatched or branch published locally.

CI preparation process tests passed4 cases; the extended E2E workflow contract
passed17 cases. SHA-pinning validation passed all26 workflows, shell syntax
passed, and workflow formatting passed. Offline PR-doc coverage remains covered
with the CI helper/workflow paths included. Publication was not authorized at that checkpoint. The user subsequently
selected the CI draft PR option, explicitly authorizing commit, push and a
draft PR to obtain clean-runner evidence despite the blocked local live gate.
No production image publication, profile rollout or merge is authorized.

Clean-runner delivery follow-up: the normal E2E workflow spent more than half an
hour in dependent runner queues before any acceptance build. A draft-only
Kubernetes Session Acceptance workflow now keeps tool setup, bounded image
build/verification, managed application builds and the five retained/new
scenarios in one disposable job. Ready PRs continue to use ordinary containers
CI; this workflow also supports later explicit manual diagnostics. Its reporter
guard requires five actually passed results, one attempt each, zero skips, zero
flakiness and no setup errors. Exact marker-bound Kind cleanup and build/blob/JSON
evidence run even on failure. Four new workflow/report contract cases were
observed red before these artifacts existed, then passed; the expanded workflow
suite passed21 cases and the preparation suite passed4. All27 workflows are
SHA-pinned. This is source verification only; live acceptance remains blocked
until the focused job executes its actual fault assertions.

The first focused workflow revision was rejected by GitHub before creating a
job: runner.temp was used in job-level env, where runner context is unavailable.
Reporter paths now live in the execution step, and result verification reads the
same runner-temp path directly. A targeted scope regression was observed red
before that correction; the expanded22-case workflow contract suite passed after
the fix. No image build or acceptance assertion executed in the rejected run.

Pinned actionlint1.7.7 passed workflow schema/expression validation after the
correction (external ShellCheck disabled); focused workflow shell blocks had
separately passed bash syntax validation. Formatting and whitespace passed.

Focused CI run37626556444 passed the bounded full-worker build and real
tool/source/browser verification. Its observed image ID was
`sha256:3201b81d6659d11d25232828693552fd109e4abee013c06dada6f39239855e16`.
All five scenarios then failed in compact-runtime fixture setup before any fault
assertion: shallow checkout made the default app version a bare Git SHA, rejected
by the canonical release-helper asset builder. The ordinary E2E build explicitly
uses a release-compatible test version. The focused managed build now uses that
same `0.0.0-e2e.<Git SHA>` convention. A regression was observed red before the
fix and now invokes the actual release asset builder with the configured version.
The five-failure JSON, blob and build log are retained privately under
`/tmp/kandev-session-resilience-focused-artifacts-37626556444`.
Kind was never allocated; marker-bound cleanup correctly did nothing.
This setup failure does not accept recovery or containment criteria.

Focused CI run37630229003 passed four of five real scenarios with zero retries,
skips or flaky results: both retained failure-recovery cases, shared task-Pod
lifecycle, and active-turn restart with one native conversation load and unique
continuation prompts. The restart receipt records retained Pod/PVC identity and
workspace checks. Its observed verified full-worker image was
`sha256:d8ef967a7e2c220d87224a47b4a16a3f98bb4c75b9c98e421a4aedadf7b70cda`.
The containment scenario failed during exact image streaming, before admission
or child-OOM assertions. Inventory records companion exit1 and a restart after
the60-second readiness window, with graceful daemon termination rather than OOM.
Companion CLI probes had no DOCKER_HOST, so they checked the default socket while
dockerd served `/run/docker/docker.sock`. Startup now explicitly uses that socket.
A process regression with absent/wrong inherited socket was observed red before
the correction; both cases now pass. Limits and accounting assertions are intact.
Evidence is retained privately under
`/tmp/kandev-session-resilience-focused-artifacts-37630229003`.
The full five-pass gate and live containment remain unaccepted.


Focused CI run37635602757 at8086f05d selected five real scenarios with retries0:
three retained cases passed; native restart and containment failed. Native
inventory records exit0/Completed after SIGTERM, so its earlier pass does not
establish ungraceful recovery. The fault now sends SIGKILL only after the same
active trace/RUNNING barrier and requires recorded exit137. Conversation,
prompt, Pod/PVC and workspace assertions remain unchanged.

The user explicitly continued after the three-attempt checkpoint. A bounded
local probe with the pinned Docker29.1.5 daemon reproduced the accounting failure
under a host cgroup namespace: the mount root has no memory.max, while the
companion's own group has its3GiB limit. Opt-in startup now resolves that group,
verifies its budget, moves only startup into its init child, enables controllers
there and places Docker descendants under its exact docker child. A process
regression was observed red before correction. Other cases refuse wrong or
unbounded parents and children outside that companion. Receipt probe paths are
companion-relative, with a separate namespace path for diagnostics.

A real64MiB Python probe now passes the actual runner receipt validator.
Independent host inspection proves ancestry beneath the3GiB companion,
64MiB memory,0.25 CPU and32 PID limits; observed child usage20.5MiB and
startup parent delta18.3MiB. Exact temporary containers/anonymous data volumes
were removed. This small daemon experiment is not full-image Kind, native
recovery or deliberate child-OOM acceptance. Private receipt:
`/tmp/kandev-session-resilience-cgroup-smoke-green.json`.

The same CI head's backend failure was the persisted full-script migration test:
it synthesized historical bytes from today's opt-in recipe and no longer matched
the known migration hash. The test now uses the actual historical fixture,
whose SHA256 matches the existing managed-script identity. Production migration
is unchanged; customized scripts stay unchanged and historical repair does not
enroll retained profiles in isolation. The original failure reproduced locally;
targeted preparation/recovery/task-Pod race tests now pass in10.168s. All25
worker recipe/runner tests pass. Live five-scenario CI acceptance remains pending;
specifications remain draft and production rollout remains excluded.


The first post-checkpoint focused run37683760716 at1a6bd851a was cancelled
before worker build or Kind allocation after Chromium dependency setup stalled
for26 minutes. Completed logs show repeated unreachable HTTP Azure archive
requests from APT's mirrorlist; Ubuntu's HTTPS archive answered release requests,
but package-index acquisition then stopped progressing. No fault scenario or
five-pass report executed. The focused disposable job now replaces only its
Ubuntu mirrorlist with the official HTTPS archive, configures30-second HTTP/HTTPS
transport timeouts and2 retries, and bounds the Chromium setup step at10 minutes.
Package authentication and the full five-scenario/zero-retry gate are unchanged.
A workflow/process regression was observed red before correction; it executes the
real setup shell with redirected system writes, requires configuration before
Playwright invocation and validates timeout syntax with the actual APT parser.
All24 workflow contracts,27 action-pinning checks, pinned actionlint and shell
syntax pass. No operator machine or production worker APT configuration changed.

---
id: "03-live-validation"
title: "Validate real imports and OAuth refresh"
status: done
wave: 2
depends_on:
  - "02-harden-bootstrap"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OPENCODE-AUTH-001
acceptance_criteria:
  - AC-EXECUTORS-OPENCODE-AUTH-001.1
  - AC-EXECUTORS-OPENCODE-AUTH-001.2
  - AC-EXECUTORS-OPENCODE-AUTH-001.5
  - AC-EXECUTORS-OPENCODE-AUTH-001.6
  - AC-EXECUTORS-OPENCODE-AUTH-001.7
  - AC-EXECUTORS-OPENCODE-AUTH-001.8
  - AC-EXECUTORS-OPENCODE-AUTH-001.10
  - AC-EXECUTORS-OPENCODE-AUTH-001.11
system_design:
  - ../../specs/executors/system-design/opencode-v2-credentials.md
---

# Task 03: Validate real imports and OAuth refresh

## Summary

Completed with the explicit natural-expiry/concurrent-refresh waiver below.
Final provider validation passed for the controller and fresh Kubernetes sessions
A/B/C, including actual replies after stop/resume. Source CLI was 2.2.5 and
destination CLI 2.2.1. The complete synthetic interruption/lost-ack matrix passed
on Docker, SSH, and Kubernetes at 2.1.2 and 2.2.1 with two independent HOMEs.
Fault injection covers the real worker HTTP/CLI boundary separately from the
controller lifecycle smoke; it does not claim controller transport fault injection.
Final command used the private OAuth inventory with `skip_natural_expiry: true`;
it reported fresh-session and post-resume responses passed, refresh waived.
The resume harness explicitly disables automatic queue dispatch before adding
and draining its prompt, avoiding duplicate dispatch races.
Disposable controllers and Docker/SSH test workers are stopped/removed. Two
previously preserved recovery pods and the private recovery credential store
remain intentionally retained following the earlier failed refresh attempt.

Produce reproducible real-CLI/executor evidence, then verify actual expiry and
concurrent refresh with the account explicitly authorized in the current lab.
Separate merge/cancellation evidence from provider token-rotation evidence.

## In scope

Add `scripts/test-opencode-credential-bootstrap.py`, an opt-in harness accepting
a private non-secret target inventory. It must drive the rebuilt controller and
workers through the real lifecycle boundary, not call only an import subprocess.
Targets identify isolated Docker, SSH, and Kubernetes test environments and
source execution context; credential values are never inventory fields.
Transfer live exports only over authenticated transport into stdin. Use memory
for sensitive comparisons and emit only non-secret assertions.

Test matrix:

- Real CLI 2.1.2 minimum and the current deployed version, recorded explicitly.
- First/second session HOME per executor, active-account preservation, destination
  token preservation, and stop/resume while source credentials are unavailable.
- Interrupt real import after observing a partial multi-account mutation in
  memory, restart the worker, then retry. An interruption before any mutation
  is not sufficient evidence. Synthetic accounts are appropriate here.
- Drop acknowledgement after successful import and retry; preserve destination
  values, selections, and completed bootstrap state.
- Controller plus sessions A/B use the authorized subscription account. Record
  natural expiry and synchronize provider-backed requests so refresh attempts
  overlap. Record actual refresh success/failure without token values.
- Bootstrap fresh session C after refresh. Require provider-backed requests from
  controller, A, B, and C to succeed. Test each path again after stop/resume.

## Out of scope

No production rollout, deletion of existing tasks/PVCs, controller-account
revocation, database editing to simulate expiry, broker implementation, push,
or GitHub comments. Remove only disposable resources created for this test.

## Acceptance

### Authorized validation exception

The user explicitly waived waiting for natural expiry after reauthentication
issued a credential with approximately ten days remaining. The natural-expiry
and successful concurrent-refresh portion of item 2 below is therefore waived,
not passed. Fresh provisioning and provider-backed responses before and after
stop/resume remain required. The harness records this exception only when the
private inventory explicitly sets `oauth.skip_natural_expiry` to `true`.

### Original criteria

1. Docker, SSH, and Kubernetes real-lifecycle smoke and interrupted/lost-response
   tests pass with reproducible commands, CLI/build versions, and redacted evidence.
2. Actual concurrent refresh and subsequent fresh provisioning are evidenced for
   the authorized account. If expiry has not occurred, attempts did not overlap,
   or credentials become invalid, record blocked/failed and stop for a decision.
3. Publish only local redacted results in the work order; raw private state never
   enters Git. Document precise recovery commands verified in the session context.

## Verification

The script is created by this task; the following is its required interface.
The operator-local target inventory contains addresses/context references only
and is populated after inspecting available infrastructure. Missing tools,
authentication, or executor access blocks the affected scenario.

```sh
python3 scripts/test-opencode-credential-bootstrap.py --targets /tmp/opencode/kandev-credential-targets.json --scenario smoke --executors docker,ssh,kubernetes
python3 scripts/test-opencode-credential-bootstrap.py --targets /tmp/opencode/kandev-credential-targets.json --scenario interrupted-import --executors docker,ssh,kubernetes
python3 scripts/test-opencode-credential-bootstrap.py --targets /tmp/opencode/kandev-credential-targets.json --scenario lost-ack --executors docker,ssh,kubernetes
python3 scripts/test-opencode-credential-bootstrap.py --targets /tmp/opencode/kandev-credential-targets.json --scenario oauth-refresh --executors kubernetes
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/opencode-v2-credentials
```

Run the synthetic matrix at the minimum and deployed CLI versions by selecting
the corresponding target inventory entries. Live OAuth validates the deployed
version; do not extrapolate its provider result to the minimum version. The
harness must fail on skipped required checks. Never put tokens in command args,
environment variables, logs, reports, or temporary transfer files.

## Files likely touched

- `scripts/test-opencode-credential-bootstrap.py` (new)
- `docs/plans/opencode-v2-credentials/task-03-live-validation.md`
- `docs/plans/opencode-v2-credentials/plan.md`
- `docs/public/executors.md`

## Dependencies

Task 02, rebuilt binaries, real executor access, CLI availability, and the
operator-authorized current lab OAuth account. Inspect `../aiaas/lab/WORKAROUNDS.md`
for operational constraints; do not copy private rollout records into this repo.

## Risks

Refresh rotation may invalidate independently stored copies, including the
live lab account. Stop on invalidation and report the recovery needed. A
synthetically shortened expiry is not actual provider expiry. Infrastructure
availability and time-to-expiry remain unverified until execution.

## Parallelism

`sequential`

## Inputs

The paired design's evidence boundary, issue 4693 maintainer feedback, current
lab workaround records, and successful Task 02 results.

## Results

The user explicitly waived natural-expiry/concurrent-refresh validation after
reauthentication produced an OAuth token with approximately ten days until expiry.
That requirement is waived, not passed; the earlier three HTTP 401 responses remain
failed refresh evidence. Fresh-session provider responses and post-restart responses
are being checked with the new credential. The source CLI was upgraded to 2.2.5;
the executor compatibility matrix above covers 2.1.2 and 2.2.1.
Task 02's HTTP timeout mismatch was fixed and its follow-up checks passed.

- Rebuilt controller started in an isolated HOME/XDG/data directory under
  `/tmp/opencode/kandev-credential-controller`, bound to `127.0.0.1:39871`.
  Health, workspace, and executor read endpoints returned HTTP 200. This is
   setup evidence only. That idle server was explicitly stopped. Subsequent
   probes start an isolated controller and stop it in a `finally` block.

- Rebuilt controller and worker completed actual SSH, Docker, and Kubernetes
  launch paths with managed CLI 2.1.2 and 2.2.1 and a synthetic account. Each reached ACP
  startup and the expected provider-authentication rejection for the fake key.
  Separate public-CLI exports verified the exact synthetic account at each
  destination with explicit session HOME/XDG paths. Kubernetes used a disposable
  validation namespace, not the existing lab deployment. These are first-store
  bootstrap checks, not proof of valid provider authentication or OAuth refresh
  concurrency.
- Actual controller `session.stop` and `session.launch` (resume intent) succeeded
  for all three executors on both CLI versions while the isolated controller's
  credential store was absent. A public CLI export confirmed the source was
  empty during each restart. Subsequent destination exports remained identical,
  including values and selection (the first SSH 2.2.1 run checked resume only).
  The source store was restored and each controller stopped in `finally` blocks.
- Live Kubernetes startup exposed status/import using the ordinary 60-second
  HTTP client despite their two-minute provisioning budget. A regression failed
  for both operations before switching them to the existing long-running client
  with a bounded two-minute context. The focused credential race suite passed
  19 tests, and Kubernetes first-store and restart checks passed afterward.
- The reusable `scripts/test-opencode-credential-bootstrap.py` now drives real
  controller smoke and invokes `scripts/opencode-credential-faults.mjs` for
  bounded worker fault probes. Earlier harness failures were diagnosed before
  recording the subsequent passing matrices below.
- The integrated interrupted-import harness passed both independent SSH homes
  on CLI 2.1.2, including controller bootstrap and restart without source.
  Docker then failed at separate fault-worker startup. The helper now removes
  inherited executor bootstrap-authentication settings from its new loopback
   worker environment. Subsequent integrated runs passed both independent homes
   on all three executors at both 2.1.2 and 2.2.1, including bootstrap, restart
   without controller credentials, destination preservation, and actual partial
   import recovery at the separate worker boundary. This is test isolation, not
    a change to executor authentication. The full lost-acknowledgement matrix also
    passed: both independent homes on all three executors at both CLI versions,
    including the controller bootstrap and restart checks.
- Direct disposable-SSH worker probes passed lost acknowledgement on CLI 2.1.2
  and 2.2.1: a proxy consumed a successful import response before dropping the
  connection; retry preserved the original value and active selection. A real
   2.1.2 and 2.2.1 import of 4,000 synthetic accounts was killed after a public CLI export
  observed partial mutation. The restarted worker reported pending, retry
  completed the store, and all previously imported entries remained identical.
  These are actual worker HTTP/CLI fault checks, separate from controller launch
    fault-path coverage. Cross-executor worker fault coverage passed; provider-backed OAuth
   refresh concurrency remains unfinished. Synthetic authentication failures are
  not provider-success evidence.

- Disposable SSH target with pinned host key is reachable. Real CLI 2.1.2 and
  2.2.1 imported synthetic credentials over stdin and preserved the original
  export after duplicate-ID imports with changed values and active selection.
  These direct CLI probes do not satisfy the real controller lifecycle matrix.
- Read-only lab source export through the installed CLI 2.2.1 found one active
  OAuth account with its recorded expiry approximately 469 minutes before the
  probe's host clock. Export was captured in memory; no credential contents were
  printed or persisted. No provider request or refresh was attempted. Actual
   overlapping refresh and subsequent provisioning remain unverified.
- The user authorized temporary TLS inspection for disposable clients only.
  The test proxy passed three concurrent authenticated TLS clients against a
  synthetic upstream, including overlapping forwarding timestamps and rejection
  of non-refresh grant types. This is proxy verification, not provider evidence.
  A subsequent memory-only source check still found one naturally expired active
    OAuth account. The provider-backed run observed three overlapping actual
    refresh requests; all three returned HTTP 401. The proxy aborted the run.
    No successful refresh, fresh-C provider response, or post-resume provider
    response was established. This does not distinguish a previously unusable
    refresh credential from a copying/rotation problem. Do not infer broker
    necessity or credential-copy safety from this result. The runner stopped its
    controller and preserved disposable worker stores and the isolated source
    recovery store. Further provider requests or reauthentication require the
    user's recovery decision; no automatic retry or account replacement occurred.
- The initial managed-npx preflight failed because the chosen controller prefix
  directory was absent. The installed `/data/.npm-global/bin/opencode2` reported
  version 2.2.1 and completed the read-only export. No lab configuration changed.

---
created: 2026-10-22
status: implemented
requirements:
  - REQ-EXECUTORS-OPENCODE-AUTH-001
system_design:
  - ../../specs/executors/system-design/opencode-v2-credentials.md
legacy_specs: []
---

# Native OpenCode V2 Credentials

## Overview

Address the six findings in [issue 4693](https://github.com/kdlbs/kandev/issues/4693#issuecomment-6481608299).
The executor system remains the owner because this contract governs credentials
crossing into isolated executor sessions. Preserve the completed initial work
as history; implement the revised bootstrap boundary before gathering live
refresh evidence. All work remains local, unstaged and uncommitted unless the
operator later authorizes otherwise. Do not push or post GitHub comments.

Confirmed decisions: authenticated sessions restart without controller
credentials, and the current Kandev lab account may be used for live OAuth tests.
No agents are delegated. Implementation and validation are complete with the
user-authorized natural-expiry/concurrent-refresh waiver recorded in Task 03.

## Scope and approach

- Runtime family/source/version travels from `resolveOpenCodeCommandOptions`
  through lifecycle and worker configuration. Credential commands no longer
  infer identity from argv. Preserve structured launch wrappers.
- Advertise and check `opencode-credential-bootstrap.v1` through `/identity`
  before controller export. Return actionable upgrade errors.
- Inspect destination state first; use secret-free pending state to distinguish
  interrupted imports from completed or pre-existing stores. Serialize inspection
  and import with the process manager's admission guards.
- Resolve source HOME/XDG explicitly and use the final session environment for
  destination operations. Document reauthentication and account-switching policy.
- Test interrupted CLI import, retry after lost response, and real executor
  paths. Provider-backed fresh-session and post-resume checks passed; successful
  expiry/concurrent refresh is explicitly waived, not claimed as passed.

No credential broker, provider database copying, node-restart restoration,
provider-history redesign, or cache-lifecycle changes are included.

## Compatibility and evidence

| Case | Intended behavior | Evidence |
| --- | --- | --- |
| Managed V2 >=2.1.2, supported wrapper, capable worker | Inspect, then bootstrap empty/pending stores | Task 02 regressions and Task 03 live CLI |
| Managed V2 below floor | Fail only selected provisioning | Runtime-selection regression |
| Native/custom V2 or unsupported wrapper | Explicit unsupported error before export | Runtime-selection regression |
| Missing/malformed worker capability | Upgrade/protocol error before export | Client/lifecycle regressions |
| V1, standalone, unselected method | Preserve existing behavior | Compatibility regressions |
| Present destination, unavailable controller auth | Restart without source export | Lifecycle test and executor smoke |
| Docker, SSH, Kubernetes | Same bootstrap boundary; separate transport evidence | Real-agentctl smoke on each executor |

## Tests

Task 02 owns new regression methods in existing `opencode_credentials_test.go`
files and launch tests. Map `.1/.7/.8` to destination state and source-call
counts; `.2/.5/.11` to pending/retry/admission behavior; `.3/.9` to runtime and
capability rejection; `.4` to V1/standalone/unselected selection; `.6/.10` to
bounded redaction and sentinel HOME/XDG isolation. Task 03 owns the executable
real-CLI harness and provider evidence, rather than a mock ACP assertion.

## E2E tests

Task 03 will add `scripts/test-opencode-credential-bootstrap.py` for explicit
real CLI and executor targets. It must start through the real controller launch
boundary and rebuilt agentctl, exercise first and subsequent session HOMEs,
stop/resume without controller auth, interrupted import, and lost response.
This is runtime/API E2E, not a new rendered UI or Playwright flow. Docker and
SSH coverage is required alongside Kubernetes; unavailable infrastructure is
reported as blocked, never counted as passed.

## Work orders

- [x] [Task 01: Provision V2 session credentials](task-01-provision-credentials.md)
- [x] [Task 02: Harden bootstrap admission](task-02-harden-bootstrap.md)
- [x] [Task 03: Validate real imports and OAuth refresh](task-03-live-validation.md)

## Risks and boundaries

Credential transfer must not overwrite refreshed destination accounts or leak
CLI output. Selected provisioning requires the advertised worker capability;
older workers receive an explicit upgrade error before source export. Existing
Kubernetes bootstrap recovery and runtime-cache capacity issues are separate
follow-ups. Demo configuration and workaround retirement are excluded.

## Verification

Task 02 precedes Task 03. Each owns exact checks and must record fresh results.
Task 01's historical counts do not certify the revised implementation.

## Verification results

Final: full affected-package tests, scoped race tests, build, changed-file lint,
and documentation checks passed. Real interruption and lost-acknowledgement
matrices passed on all three executors at both tested CLI versions. Actual
provider replies passed before and after resume, including fresh session C.
Successful concurrent refresh remains explicitly waived after reauthentication,
not passed; the earlier overlapping attempt returned three 401 responses.
Task work orders retain detailed chronological evidence and final outcomes.
The baseline/design-only entries below are historical.

Rebase baseline: `0f1d3d011` on upstream `edebfa524`. Focused race tests passed
295 cases in seven packages. Installed golangci-lint found four issues: two
unchecked channel-return errors and two string-constant findings. Task 02 owns
those fixes, now completed.

Design-package checks on 2026-10-30: `python3 scripts/list-docs.py validate`
passed (421 decisions, 1850 specifications); `python3 scripts/lint-spec-files.test.py`
passed 36 tests; `python3 scripts/lint-spec-files.py --all` and
`git diff --check` passed. All three work-order frontmatter fields, requirement
and acceptance IDs, design references, and local links were checked. The PR
coverage evaluator requires a GitHub PR/event context; it was not run because
no PR is authorized or created. Files remain unstaged and uncommitted.

## Live validation risks

Actual expiry may require waiting; do not replace it with a synthetic timestamp
edit. Overlapping refresh may invalidate other account copies. Record this
outcome and stop for a decision if observed. Do not deploy over the running lab
controller simply to obtain evidence; use isolated test instances configured
from the authorized account. Sensitive lab state stays outside tracked files.

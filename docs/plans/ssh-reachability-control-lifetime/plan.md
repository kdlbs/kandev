---
created: 2026-10-06
status: implemented
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-002
system_design:
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
legacy_specs: []
---

# Implementation Plan: SSH reachability control lifetime

## Overview

Keep settings load errors and pending probe controls with their committed
executor visit. One sequential work order adds transport-boundary regressions,
repairs the existing hook, and verifies real provider/card behavior. ROOT reviewed the four-file package and released implementation in the same
primary session. Task 01 is implemented; hosted delivery and merge remain
separate checkpoints.

The executor system owns this vertical correction because executor availability
and its settings controls already belong to
[REQ-EXECUTORS-SSH-REACHABILITY-002](../../specs/executors/requirements/ssh-reachability.md).
Criteria .3, .5, .6 and .10 remain unchanged. Added .13 and .14 specify the
missing visit-local lifetime and pending-control behavior. The paired
[design](../../specs/executors/system-design/ssh-reachability-surfaces.md#local-settings-request-lifetime)
defines the implementation boundary. No new feature, UI requirement or ADR is
needed: this is a local correction using an established lifecycle pattern.

## Confirmed evidence and root cause

At inspected HEAD `c373ba436c3451f046acd921d5de9a081977a2a7`, the hook,
card and API blobs match ROOT's proof base
`05c41b11e830a9861534200949c3bbac473b57f7` exactly:

| File | Blob |
| --- | --- |
| `apps/web/hooks/domains/settings/use-ssh-reachability.ts` | `96befd84b37a17185572582dabf251c3d31420b0` |
| `apps/web/components/settings/ssh-reachability-card.tsx` | `802d655f88df15515975effc56fb4b4b40edd4e5` |
| `apps/web/lib/api/domains/ssh-api.ts` | `946ff3eac3dab344b53f341240ba676bb499be7f` |

The hook resets `seqRef` to zero on executor switch. Deferred A GET sequence 1
can therefore match B GET sequence 1 and publish A's load error under B.
`probeNow` has no lifetime guard, and the switch never resets `probing`, so A's
pending probe disables B's actual rendered action. Success/catch/finally and
retained actions also lack committed store/instance ownership.

Accept the independently authored proof without replay, copying/importing it
as permanent tests, mutation or deletion:
`/tmp/kandev-root-ssh-reachability-lifetime-next-candidate.test.tsx`, readonly
0400, SHA256 `17a23ee87a6f7986e254b331c72cd9809a5f9951ad609f9f908beeeb6f0dd22a`;
classification `/tmp/kandev-root-ssh-reachability-proof-classification.json`.
ROOT reports original32379/f058d2/369042 ACTUALLYJOINED exit 1 at 18:58:19Z,
6.141s: two causal failures, two positive controls PASS (current GET failure
renders not-known; own pending probe disables until completion). Real
`StateProvider`/AppStore, hook, card and SSH APIs ran with a transport-only
partial `fetchJson` mock. Scratch and owned processes are reported gone.
This package's source inspection independently confirms the same cause; it
does not claim a new reproduction run.

## Scope

### In scope

- Existing hook's committed executor/store/instance lifetime; initial/cadence
  load and probe admission, success, catch, finally and retained callbacks.
- Current-visit errors and pending controls, preserving keyed store evidence.
- Meaningful deferred-response tests through real hooks/provider/card/API,
  including cleanup before passive effects and same-executor positive controls.
- Exact focused checks, coverage mapping, normal hooks and gated delivery.

### Out of scope

- Store shape, `updated_at` reconciliation, HTTP/WS contracts or producers.
- Backend/remote probe cancellation, coalescing, completion, persistence,
  security, launch gating, feature defaults or staleness cadence.
- Generic ownership frameworks, global error/retry policy or unrelated hooks.
- New markup, UI copy, layout, navigation, touch geometry or breakpoints.
- Delegation, new tasks/sessions/tabs, proof replay, main-only rebases,
  synthetic tests, bypassed gates or foreign-resource cleanup.

## Technical approach

Reuse the local committed-callback and `useLayoutEffect` cleanup pattern from
`apps/web/hooks/domains/office/use-routing-preview.ts`; consult
`use-session-search.ts` for identity-separated visits. Keep it inside
`useSSHReachability`, using one committed identity and non-reused generation
per lifetime. Check admission before issuing transport and settlement before
every local/store write. Reset local controls on commit; never clear store
records or mutate an active ref during render. Keep the latest GET request
guard and independently owned current probe pending admissions. A retired
finalizer cannot clear current pending controls, and a prior A visit cannot
revive after A-to-B-to-A or StrictMode replay.

Obsolete hook HTTP results are dropped, while separately admitted HTTP and WS
evidence still reconciles by executor key and `updated_at`. Preserve current
success/error behavior and store timestamp arbitration. No new ordering policy
between GET and probe is part of this fix. Admitted transports continue to
settle; retiring a hook never aborts or changes a backend probe.

Existing package `ssh-host-reachability` has eight completed work orders and
records its original container E2E evidence. It has no pending lifetime repair;
its historical results/counts are unchanged. This package owns the new
regressions and validation results, referencing the same authoritative pair.

## ASCII UI preview

UI-01: Settings > Executors > SSH, navigating A to B while A is pending.
The existing card renders its existing localized text. Labels below describe
states; they add no product copy.

```text
Before: B visit inherits A work      After: B controls belong to B
+--------------------------------+ +--------------------------------+
| Reachability [Probe now: off]   | | Reachability [Probe now: on]    |
| A load rejection: not-known    | | B record, or B not-known only  |
+--------------------------------+ +--------------------------------+
                                    B starts its own probe:
                                   +--------------------------------+
                                   | Reachability [Probe now: off]   |
                                   | B's accepted record retained   |
                                   +--------------------------------+
```

Desktop places the action beside the heading. Phone uses the existing
`SettingsCardHeader` column, placing it below the heading. There is no new
fixed region or scroll owner. Required change: current visit alone determines
error/pending state (criteria .13/.14); spacing is illustrative. State remains
text, existing touch controls remain intact, and no viewport-dependent logic
changes. Targeted real rendered component tests satisfy `/mobile-parity`'s
pure state/data exception; no new mobile Playwright or geometry check is needed.

## Tests and acceptance coverage

Write new tests independently; do not copy the ROOT proof. Prefer one new
`apps/web/hooks/domains/settings/use-ssh-reachability.lifetime.test.tsx`
containing a small real-hook reader, real provider/store capture and rendered
card flows. Partial-mock only `@/lib/api/client::fetchJson` using `importOriginal`;
real SSH GET/POST functions must run and request paths/methods must be asserted.
Existing card, store and API suites stay in the focused run.

| Criteria | Proposed named regression groups and assertions |
| --- | --- |
| .13, .10 | `retired initial load cannot mark B not-known`, `retired refresh cannot publish after A-B-A`: defer A and B, settle obsolete success/reject in both orders; inspect local error, rendered host/state and each keyed store record. |
| .13 | `retained load/probe actions are inert after commit cleanup`: invoke captured callbacks from layout cleanup/commit before passive effects; assert zero extra GET/POST and no state writes. Include pending settlement after unmount and same-executor remount/StrictMode replay. |
| .14, .5 | `B can probe while A is pending`: actual B card button is enabled, click it, assert POST targets B, B disables during its own probe, A finally cannot release B, then B settles and re-enables. Include obsolete probe success/reject. |
| .13, .14 | `independent owners keep independent controls`: same/different executors within one store and separate real providers; retiring one cannot mute another's success or pending state. |
| .3, .13 | `accepted keyed evidence survives retirement`: seed or push newer A/B records through real store action, settle obsolete HTTP, and assert records remain; current older success cannot overwrite newer record/reset but clears its own error. |
| .10, .14 | `current load/probe failures and success remain visible`: initial and refresh failure without a record shows not-known, cached record remains on failure, own success clears error and renders fields; newest same-executor refresh supersedes older failure/success. Overlapping current probe finalizers keep action disabled until own admissions settle. |
| .4, .11, .12 | Existing card cadence/staleness, translated record/state and shared viewport controls stay unchanged; record concrete passing test names in Results. |

Use deferred promises, React `act`, scoped rendered assertions and captured
real store APIs; settle every deferred test request and clean timers. Tests must
exercise public consumers and causal outcomes rather than mirroring an owner
predicate or hiding the race with immediately resolved mocks.

## E2E evidence and public documentation

Existing `apps/web/e2e/tests/ssh/reachability.spec.ts` in project `containers`
proves the broader SSH feature against a real sshd; its original plan records
two passing cases. It is historical evidence, not newly run proof of this race.
The new real hook/provider/API/rendered-card boundary tests prove this local
correction; no Docker/backend rebuild or broad E2E run is required.

Docs audit searched `docs/public`, root `README.md`, `docs/screenshots.md`,
specs and decisions for SSH/reachability/control wording. The explanation in
`docs/public/executors.md` already describes per-executor state and Probe now;
`docs/public/configuration.md` describes unchanged interval semantics. No public
docs/screenshots/copy change is needed. Internal owning docs are amended.

## Work orders

- [x] [Task 01: Bind local SSH reachability controls](task-01-bind-local-controls.md)

Sequential. No dependency work order, no delegation.

## Verification results

Design documentation checks passed on 2026-10-06:

- `python3 scripts/list-docs.py validate`: 357 decisions and 1408 specifications,
  exit 0 (original handle `99073e`).
- `python3 scripts/lint-spec-files.test.py`: 36 tests, exit 0 (`ac0433`).
- `python3 scripts/lint-spec-files.py --all`: all specification files passed,
  exit 0 (`82d037`).
- `git diff --check`: exit 0 (`f8fda1`).

All four commands completed synchronously; no running process handle remains.
Exactly the existing requirement/design pair and this manifest/order changed.
Requirement size is 19,939 bytes, below its 20 KiB limit. Design is 16,366 bytes,
below its 32 KiB limit. Owning specs are active/current; Task 01 is done following ROOT review/release.

Implementation checks passed on 2026-10-06: 63 focused tests (27 new lifetime
cases), changed ESLint, format check, project typecheck, i18n check/ratchet,
catalog and spec lint, 36 spec-linter tests, 62 public-doc validator tests,
47 public pages and diff check. Exact commands and coverage are in
[Task 01 results](task-01-bind-local-controls.md#results). All originals are
ACTUALLYJOINED with owned groups gone; receipts/logs remain privately under
`/tmp/kandev-child58`. Normal commit/push/ready PR and hosted gates follow in
this primary session; merge requires ROOT's later serial interrupt.

## Risks

- Passive-only cleanup or render-time ref mutation leaves commit-boundary gaps.
- Executor ID equality alone revives old A work after A-to-B-to-A.
- A single pending boolean can be cleared by another request's finalizer.
- Mocking the hook/API/provider can conceal real store/card wiring failures.
- Shared-store evidence must remain independent of local controls.
- Setup/resource/timeouts or out-of-scope unknown failures require a ROOT
  checkpoint, not blind retries, cache wipes or foreign-process termination.

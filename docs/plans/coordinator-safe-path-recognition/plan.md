---
created: 2026-10-06
status: implemented
requirements:
  - REQ-COORDINATOR-COPILOT-004
system_design:
  - ../../specs/coordinator/system-design/copilot-panel.md
legacy_specs: []
---

# Implementation Plan: Safe Copilot Path Recognition

## Overview

Keep the copilot reset effect safe when a workspace or coordinator component
cannot be decoded, while retaining the exact identity and state behavior of
valid Needs you and Queue paths. One sequential work order adds causal helper
and mounted bridge regressions, applies the local recognition correction,
and verifies only the affected boundary. This package is design only; its
existence does not authorize implementation, heavy checks or merge.

Coordinator owns the copilot slot lifecycle. Reuse
[AC-COORDINATOR-COPILOT-004.12](../../specs/coordinator/requirements/copilot.md#req-coordinator-copilot-004-copilot-panel)
and the existing [panel design](../../specs/coordinator/system-design/copilot-panel.md#path-reset-boundary).
There is no new router, ownership rule or architectural decision.

## Grounded defect and evidence

At current head `f289f159f2764bf35a39b7387b58dd4cdd8781f2`, the helper's
regex accepts a workspace without decoding it and calls `decodeURIComponent`
on the coordinator capture without a guard. The shell-mounted bridge calls
that helper in its pathname effect before `keepOnlyFor`. A decoding exception
interrupts the effect; an invalid workspace with a valid coordinator returns
the old coordinator identity and retains stale state. The SPA resolver already
uses `safeDecodePathSegment` for both identities and declines invalid routes.

Accepted ROOT proof at `670847f0a48cf36c14bdbd929a2fb96a3a565eea`:

- Native handle `95895`, actually joined exit 1 in 5.183 seconds, three causal
  failures and two passing controls. `%` and `%E0%A4%A` coordinator segments
  throw through the mounted bridge and replace its mounted marker with the
  error boundary fallback. Workspace `ws%ZZ` with valid `co-1/queue` retains
  the prior open/chip/draft. Encoded valid identity and ordinary departure
  controls pass.
- Read-only archive `/tmp/kandev-coordinator-route-decoding-wire-repro.test.tsx`,
  SHA256 `9248c999cecd81d9c8b000cb1a4c988b459d8e67bf79aa1524d2fcc4ee41db89`;
  accompanying `/tmp/kandev-root-coordinator-route-bridge-proof-receipt.json`,
  `-classification.json`, `-native.json` and `.log` remain owned by ROOT.
- Source comparison against that baseline found no changes to the helper,
  bridge, native router, safe decoder or SPA route resolver. The proof was
  inspected, never replayed or changed. Its corrected fixture mounts the real
  bridge/store/native pathname subscription and decoder, not the full SPA.
  Earlier native `65009` failed importing missing generated release notes
  before tests: no bug verdict and no full-SPA coverage from that run.

## Scope

### In scope

- Local safe recognition of both encoded path components, with unchanged
  public route grammar and helper return type.
- Meaningful permanent helper tests and mounted bridge/store/native pathname
  tests, with independent preservation, reset and close/reopen controls.
- Requirement/design traceability, public-guidance audit, pure-state mobile
  exception, and bounded affected verification after explicit authorization.

### Out of scope

- Global router/framework/policy, new fallback or navigation behavior,
  `useParams` decoding, or a generic malformed-URI sweep.
- Backend, transport, coordinator workflow, conversations, caches, flags,
  persistence, membership validation, or a workspace-keyed store model.
- Layout, markup, copy, mobile interaction, screenshots, browser/build/E2E,
  full-SPA imports, generated setup repair, installs or product tests now.
- Delegation, new sessions/tabs, model switches, commit/PR/hooks or merge now.

## Technical approach

Change only `apps/web/hooks/domains/coordinator/coordinator-path.ts` in
production: capture workspace and coordinator separately, decode each once
with the existing `safeDecodePathSegment`, and return the decoded coordinator
only when both results are valid. Keep the anchored optional Queue/trailing
slash grammar. Do not modify the shared decoder without a newly grounded
causal need. The bridge and store should need no production edits.

The sole production consumer is `CoordinatorCopilotResetBridge`, mounted in
`apps/web/src/app-shell.tsx`. It consumes the real `usePathname` subscription
and calls `keepOnlyFor`. The single slot resets `draftsSwept` along with its
entry on departure. `app/coordinator/copilot/use-coordinator-copilot.ts` uses
`setOpen` for normal close/reopen, sweeps composer storage after a slot reset,
and removes gone coordinators independently. These consumers stay intact.

| Input shape | Recognition and slot outcome | Planned evidence |
| --- | --- | --- |
| Same coordinator, Needs you/Queue, optional final slash | Exact once-decoded coordinator; complete entry preserved | Helper and mounted native transitions in both directions |
| `%`, `ws%ZZ`, incomplete UTF-8 `%E0%A4%A` in either component | `null`; previous slot cleared; marker remains mounted | Independent malformed-component tables and mounted boundary assertions |
| Valid `ws%2Fone`, `co%2F1`, `co%25`, `co%252F1`, Unicode | Exact decoded identity; literal percent/slash content preserved without a second decode | Explicit helper outputs and fully seeded real store snapshots |
| Other coordinator, generic coordinator page, missing components, unsupported suffix, ordinary page/root | `null` or other identity; previous slot cleared | Independently reseeded departure cases |
| Close/reopen on a recognized current path | Chip/draft preserved; only open changes | Mounted real store actions plus existing store controls |

## Tests

All rows serve `AC-COORDINATOR-COPILOT-004.12`. Planned tests are pending,
not coverage claims:

| File | Planned tests and assertion value |
| --- | --- |
| `hooks/domains/coordinator/coordinator-path.test.ts` | `rejects undecodable workspace and coordinator components`; `decodes valid identities exactly once`; `accepts only Needs you and Queue with optional trailing slash`. Invoke the production helper with explicit expected identities/null, never a copied regex/parser. |
| `components/coordinator-copilot-reset-bridge.test.tsx` | `clears malformed paths without replacing the mounted application`; `preserves the whole slot across native Needs you and Queue transitions`; `clears each independent departure`; `keeps chip and draft across close and reopen`. Use the actual bridge, native router subscription, history plus popstate, real Zustand store and React error boundary. Assert coordinator ownership, open, chip including ref, draft and draftsSwept as applicable. |
| `hooks/domains/coordinator/copilot-store.test.ts` | Run existing ownership, null-reset, draftsSwept, and setOpen-only controls as the independent store boundary. No store implementation change is planned. |

Replace the current bridge test's mocked pathname fixture with native history
events. Save and restore initial location; cleanup subscriptions/rendered trees,
real store state and any narrowly scoped console spy after each case. Reseed
before each invalid path so an earlier reset cannot hide a later failure.
Do not import `spa-routes.tsx` or the full application to prove this boundary.

## Mobile and public documentation audit

This correction normalizes state at the existing viewport-independent bridge.
It adds no rendered layout, touch, scroll, navigation, copy or breakpoint
behavior. The mobile-parity pure-state exception permits targeted helper and
mounted component evidence. No new ASCII UI composition or mobile Playwright
test is needed. No browser/E2E/full-SPA claim is made.

Public audit searched `docs/public/**`, root `README.md` and
`docs/screenshots.md`. `docs/public/feature-status.md` describes Coordinator
as an attended, flagged core page; other Copilot hits concern the agent CLI.
The feature description, route structure and operator instructions remain
accurate. Internal docs are updated; no public guide change is needed.
The earlier workspace-coordinator task-11 package owns the panel rollout;
its pending status is retained, and this focused repair does not claim or
complete that larger implementation record.

## Verification strategy

Design checks are limited to the catalog, specification linter's actual
36-test script, all-spec lint, exact references/documentation-coverage preflight,
whitespace and four-file inventory. Each gets a 60-second absolute cutoff,
owned PID/group, original native receipt and log, actual join and gone proof.
No package installation or product check is authorized in this turn.

Later work requires ROOT's explicit same-primary implementation interrupt and
exclusive LOCAL-HEAVY grant. The exact targeted Vitest selection and affected
eslint/typecheck/i18n/docs/coverage commands are in
[Task 01](task-01-safe-copilot-path.md#verification). Only the approved original
targeted run is active at a time, with Node 4 GiB, 120-second timeout and
10-second kill grace. Capture RED before correction and GREEN after correction;
do not substitute the accepted disposable proof for permanent evidence.

Timeout, resource, transport, unknown or out-of-scope failures checkpoint ROOT
without automatic retry. Only causal repairs to the owned fixture or affected
lint are routine. Record and join every original handle and prove its owned
processes/group gone before another heavy command. One pinned pnpm 9.15.9
frozen install from `apps/` is conditional on absent dependencies and the
later exclusive grant; preserve any existing installation and lockfile.

## Work orders

- [x] [Task 01: Safe copilot path recognition](task-01-safe-copilot-path.md)

One work order, sequential, no dependencies or delegation.

## Verification results

Task 01 is done. The helper now reuses existing `matchDouble` locally; no
shared decoder, bridge/store/router, public route, layout, copy or viewport
contract changed. Permanent real-boundary RED reproduced 12 causal failures
with 44 controls passing; GREEN passed all 56 tests in three files. Affected
ESLint, normal project typecheck, i18n, owning catalog/catalog validation,
all-spec lint and actual changed-path documentation coverage passed. Full
commands, counts, original receipts and setup recovery are in
[Task 01 results](task-01-safe-copilot-path.md#results).

The original direct tsc exit 2 is preserved as setup failure. ROOT authorized
one normal project typecheck with its tracked pretypecheck generators; the
corrected original passed and created only two previously absent ignored JSON
assets. The earlier design Node setup failure and corrected exempt check are
also preserved. Actual implementation coverage is covered with no errors;
this is separate from the design-only exemption and later hosted exact-head gate.

The design catalog/spec lint and all 36 linter-script tests passed previously.
The accepted ROOT proof was never replayed or modified. Every original command
has an upfront PID/group/start/cutoff/argv/log receipt and actual join/gone proof
in `/tmp/kandev-copilot-implementation-5573589e-7i_1pojv/`. No broad tests,
full-SPA fixture, browser/build/E2E or global malformed-route claim was added.

Task completion remains pending verified merge and joined owned cleanup.
Normal hook/publication and hosted review/CI results are recorded in the live
Kandev task plan, with a separate ROOT merge grant required.

## Risks

- Decoding twice would reject valid literal-percent identities or turn encoded
  text into a different identity; assert explicit once-decoded values.
- Capturing the workspace changes regex indexes; tests must cover both views
  and both invalid components with fresh slot seeds.
- A mocked pathname or copied parser could pass without exercising the effect;
  use the actual native subscription and error boundary.
- Full SPA imports can fail on unrelated generated assets. Those failures are
  setup limitations, not causal regression evidence or permission to repair setup.
- `useParams` remains separately unproved. Do not broaden the fix or report a
  universal malformed-route guarantee.

---
created: 2026-09-10
status: done
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
  - REQ-PLUGINS-ISOLATED-WEB-APPS-012
  - REQ-CANVASES-LOCAL-CREATION-001
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-003
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-007
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
  - ../../specs/canvases/system-design/local-creation-authority.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
legacy_specs: []
---

# Implementation Plan: Canvas runtime and permission fixes

## Overview

Make the published canvas load through any same-origin Kandev hostname, remove
duplicate consent for owner-authorized first publication, show accurate startup
state, and make release review readable on desktop and phone.

Execute four work orders sequentially. Correct the confirmed hosting blocker
first. Add initial creation authority next, then startup detection, then the
review surface. Each order has its own regression evidence; this package does
not authorize implementation, deployment, or changes to the reported live task.

## Evidence and conformance

The investigation used task `00d2d513-3601-4413-9fe2-024b0b23cb17`, canvas
`65421ec3-89ad-4886-a5d6-750c217d26b5`, release
`cf3b3eb8-a562-4dfb-83c6-ef44072f7364`. On 2026-09-10, the public runtime
document returned HTTP 200 but allowed only localhost and Tauri ancestors.
The actual parent, `https://kandev.cfl.tools`, was absent. Approval succeeded
at 20:32:30 UTC; context, task, workflow, and step endpoints then returned
successfully. No end-to-end success after a policy fix has yet been observed.
The retained diagnostic bundle has truncated historical logs; absence of a log
entry was not used to infer success.

| Finding | Classification | Authoritative criteria |
| --- | --- | --- |
| Custom public origin blocked | Implementation/design gap | `AC-PLUGINS-ISOLATED-WEB-APPS-007.8`, `.10` |
| Initial owner permission gate | Intended behavior change | `AC-CANVASES-LOCAL-CREATION-001.1` through `.7`; amended canvas `.003.4/.5` |
| URL/load event reported Ready | Missing startup contract | `AC-PLUGINS-ISOLATED-WEB-APPS-012.1` through `.4`; canvas `.007.5/.6` |
| UUIDs, duplication, hidden actions | Existing UX gap plus measurable criteria | canvas `.006.8/.9`, `.007.4/.7/.8` |

The plugin system owns shared runtime policy and lifecycle signalling.
Canvases owns local creation authority and release-review outcomes. The paired
designs link these boundaries instead of creating a separate UI specification.

## Scope

### In scope

- Host-owned `frame-ancestors 'self'` plus existing exact launcher/Tauri origins.
- Two custom HTTPS hostnames, localhost, and unrelated-parent browser evidence.
- Single-use recorded owner authority for a new draft's first release.
- Exact initial grants for supported task data, events, state, and HTTPS origins.
- Preserved later review, revocation, migration, and promotion behavior.
- Host-provided startup detection for retained and new packages.
- Readable release/source labels, grouped permissions, and fixed review actions.
- Desktop and phone verification, authoring guidance, and public documentation
  updates during implementation.

### Out of scope

- Marketplace/import implementation, package export, or cross-instance sharing.
- A public-origin setting or automatic trust of forwarded request headers.
- Unlimited framing, `allow-same-origin`, or a privileged iframe SDK.
- Ongoing automatic approval for later permission increases.
- Repairing, republishing, or changing permissions on the reported live canvas.
- Workflow-wide data access for task canvases or historical transition APIs.
- Feature-flag identities, profile defaults, and rollout changes.

## Technical approach

### 01: Same-origin embedding

`webapp.BuildContentSecurityPolicy` adds the CSP keyword separately from the
exact-origin normalizer. Keep `FrameAncestorsForConfig` and provider wiring for
explicit development/desktop exceptions. The public browser origin comes from
the response URL through CSP semantics, not request-header parsing. Reuse the
runtime's relative capability URLs. No database change is involved.

### 02: Owner-authorized first publication

Implement the new [creation design](../../specs/canvases/system-design/local-creation-authority.md).
The adapter records creation authority in the canvas creation transaction.
The first publish consumes it with grants and activation in the existing
authority-fenced transaction. Existing drafts have no authority row. Neither
source metadata nor workspace ownership of an import is sufficient.

This is a deliberate initial delegation, including exact HTTPS origins. Later
changes remain reviewable. Retain legacy/manual approval fixtures and add
separate real owner-created fixtures; do not delete approval coverage just
because the creation happy path no longer needs it.

### 03: Runtime startup

Insert a bounded host bootstrap into the served entry representation without
changing stored artifacts. `WebAppFrame` uses a per-mount probe/result exchange
with exact-window and nonce checks. Its 15-second deadline is distinct from
capability expiry. `CanvasHostSurface` mounts during `loading_runtime`; a
descriptor alone must not set Ready. Failed startup tears down the frame and
shows recovery. Preserve theme-before-reveal and immediate authority teardown.

### 04: Release review

Extend authorized HTTP projections with optional readable source labels.
Keep IDs as action identities, not visible copy. Use one selected release,
defaulting to pending then active, one permission summary, and fixed actions.
Distinguish Active, Previous, Pending review, Invalid, and Unavailable.
Promotion reuses permission/provenance copy without changing confirmation
preconditions. Reuse localization catalogs for all host copy.

## ASCII UI preview

Structural requirements are selection, hierarchy, scroll ownership, and fixed
actions. Text and spacing below are illustrative; all UI copy is localized.

### UI-01: Canvas startup, task panel or focused route

Desktop and phone share the state sequence. Phone keeps its existing focused
route and visible Back/overflow controls.

```text
Before: Live workflow | Ready
        [ browser error page inside frame ]

After:  Live workflow | Loading...
        [ covered frame awaiting startup ]
          | acknowledgement       | deadline/failure
          v                       v
        Ready                 Canvas could not start
        [ application ]       [Retry] [Releases]
```

`AC-CANVASES-AGENT-WEB-APPS-007.5/.6` and plugin `.012.1-.4`.
Host controls remain outside the failed frame. Do not diagnose CSP from timeout
alone. Offline and permission-review states keep their existing meanings.

### UI-02: Desktop Releases and permissions, pending release selected

Entry: canvas host Releases and permissions, or the equivalent workspace action.

```text
Before: [ release UUID                   Pending ]
        [ Declared permissions                  ]
        [ Missing permissions, repeated         ] <- 288px scroller
        [ source UUIDs ... Approve/Reject below  ]

After: +-------------------------------------------------------+
       | Live workflow: Releases and permissions           [X] | fixed
       | Release [10 Sep, 21:27 v]          Pending review      |
       | Created by your agent                                |
       | From: Recreate live workflow diagram canvas           |
       |-------------------------------------------------------|
       | Requested access in this task                         | single
       | Read tasks                                      New   | scroll
       | Read workflows                                  New   | region
       | [Additional details, if needed]                       |
       |-------------------------------------------------------|
       | [Reject]                          [Approve and open]  | fixed
       +-------------------------------------------------------+
```

Target maximum width 48rem, bounded by available width and height. At
1280x720, the simple two-permission case needs no scroll. Selecting a previous
release replaces the footer with Roll back; the current active release has
Close. Empty history and failed fetch have readable states; failed mutations
keep selection and show an inline error. Pending review is not a red validation
failure. Missing source labels say unavailable, never a UUID.

### UI-03: Phone review, pending release selected

```text
+----------------------------------+
| < Back   Releases and permissions| fixed
| Live workflow                    |
| Release [10 Sep, 21:27 v]         |
| Pending review                   |
|----------------------------------|
| Created by your agent            | single
| From: Recreate live workflow...   | scroll
|                                  | region
| Access in this task              |
| Read tasks                  New  |
| Read workflows              New  |
| ... additional permissions ...   |
|----------------------------------|
| [Reject]      [Approve and open]  | fixed, >=44px targets
|          bottom safe area        |
+----------------------------------+
```

This review is a full-height mobile surface because it can contain detailed
permissions and release history. The existing host action chooser remains an
inset drawer. Reuse `task-layout.tsx`'s focused composition and
`mobile-menu-sheet.tsx`'s fixed header/internal scroll/safe-area mechanics.
Shared selection and mutations prevent desktop/mobile behavior drift.
Test at 390x844, 767px, and 768px widths; focus returns to the opener.
UI-02/03 map to canvas `.006.8/.9`, `.007.4/.7/.8`.

## Tests

Proposed regression names are implementation targets, not existing evidence.

| Work order / criteria | Regression and location |
| --- | --- |
| 01 / plugin `.007.8/.10` | `TestBuildContentSecurityPolicyIncludesSelf` in `policy_test.go`; runtime headers in `runtime_test.go` |
| 02 / local creation `.001.1-.7` | `TestCanvasCreationAuthorityFirstPublish`, identity/migration/rollback/race variants in canvas `authoring_test.go`, `canvas_test.go`, backendapp `services_canvas_test.go`, instance `store_test.go` |
| 02 / preserved canvas `.003.4/.5` | Keep `TestPublishPackageFirstReleaseRequiresMatchingGrants` for no-authority instances and later-expansion approval tests |
| 03 / plugin `.012.1-.4`, canvas `.007.5/.6` | `TestRuntimeStartupBootstrapPreservesArtifact` in `runtime_test.go`; nonce/timeout tests in new `web-app-startup.test.ts`; host loading/retry tests in existing component suites |
| 04 / canvas `.006.8/.9`, `.007.4/.7/.8` | Label authorization in backendapp canvas tests; copy/view-model tests and `canvas-lifecycle-dialogs.test.tsx`; viewport assertions in desktop/mobile E2E |

## E2E tests

- New `tests/canvas/canvas-host-origins.spec.ts` (`chromium`): an isolated TLS
  reverse proxy serves actual runtime responses through `canvas-a.example.test`
  and `canvas-b.example.test`; same-origin frames execute under each host.
  `unrelated.example.test` framing canvas A is blocked, including a nested
  foreign ancestor. Header forwarding must not alter trust. Preserve CSP in
  the proxy and assert actual rendered content. Use scoped test certificates
  and browser-local routing/proxy configuration; never edit system DNS or
  contact those names externally. Reuse TLS fixture mechanics from
  `helpers/plugin-git-credentials.ts`, without its Git or Docker machinery.
- `tests/canvas/plugin-canvas.spec.ts` (`chromium`): first owner publication
  opens without approval; a later permission increase still requires review;
  startup failure, retry, retained package, and desktop dialog geometry.
- `tests/canvas/mobile-plugin-canvas.spec.ts` (`mobile-chrome`): automatic
  first open, startup recovery, full-height review, long permissions, fixed
  44px actions, source fallback, focus return, and no horizontal overflow.
- Preserve promotion, rollback, task scope, live data, theme, and revocation
  scenarios already in both files. The managed runner rebuilds product code.

Run commands are in each work order. Fresh worktree prerequisite:
`(cd apps && pnpm install --frozen-lockfile)`. Run desktop and mobile commands
separately with one worker per shard and retries disabled. Start observers
before stimuli; use causal waits and fake clocks for deadline tests.

## Work orders

- [x] [Task 01: Support same-origin canvas embedding](task-01-same-origin-embedding.md)
- [x] [Task 02: Authorize initial owner publication](task-02-initial-owner-publication.md)
- [x] [Task 03: Detect canvas startup](task-03-runtime-startup.md)
- [x] [Task 04: Make release review readable](task-04-release-review.md)

## Related delivery records

The [original package](../plugin-backed-canvases/plan.md) is historical completed
work. This repair supersedes its framing, load-as-ready, initial-approval, and
review-layout assumptions, not its recorded test results. The
[UX follow-up](../plugin-backed-canvases-ux-follow-up/plan.md) retains its pending
external ACP evaluation; this repair changes appearance reveal timing but does
not complete that evaluation. The
[MCP prompt package](../mcp-discovery-canvas-prompts/plan.md) retains its discovery
scope. Its conditional permission-review wording remains valid.

## Verification results

Design validation on 2026-09-10:

- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Package link/reference check: all local Markdown targets exist; all 40
  frontmatter requirement/acceptance references exist in the specification set.
- `git diff --check`: passed.
- `git status --short -- docs/plans/canvas-runtime-permission-fixes`: confirmed
  the new manifest and all four work orders are present for staging.

Task 01 implementation verification completed on 2026-09-10:

- `(cd apps/backend && go test ./internal/plugins/webapp/...)`: passed, 30 tests.
- `(cd apps/web && pnpm e2e:run --project chromium tests/canvas/canvas-host-origins.spec.ts -- --retries=0)`: passed, 1 test.
- `node --test scripts/validate-public-docs.test.mjs`: passed, 62 tests.
- `node scripts/validate-public-docs.mjs`: passed, 46 published docs pages.
- `git diff --check`: passed.

Task 02 implementation verification completed on 2026-09-10:

- `(cd apps/backend && go test ./internal/canvas/... ./internal/plugins/instances/...)`: passed, 49 tests.
- `(cd apps/backend && go test ./internal/backendapp -run 'Canvas' -count=1)`: passed, 23 tests.
- `(cd apps/backend && go test ./internal/mcp/canvasskill ./internal/mcp/handlers -run 'Canvas|Bundle|Scaffold' -count=1)`: passed, 12 tests.
- `(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/canvas/plugin-canvas.spec.ts -- --retries=0)`: passed, 2 tests.
- `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0)`: passed, 3 tests.
- Public-doc tests/validator, specification lint, and `git diff --check`: passed.

Tasks 03 and 04 implementation verification completed on 2026-09-11:

- `(cd apps/backend && go test ./internal/canvas ./internal/plugins/webapp ./internal/backendapp ./internal/plugins/instances)`: passed, 955 tests.
- `(cd apps/web && pnpm exec vitest run components/plugins/web-app-frame.test.tsx components/plugins/web-app-startup.test.ts components/settings/canvas-lifecycle-dialogs.test.tsx lib/canvas-permission-copy.test.ts)`: passed, 30 tests.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/web && pnpm run i18n:check)`: passed.
- Scoped frontend ESLint and `git diff --check`: passed.

## Risks

- Initial local code receives declared writes and network access through the
  owner's creation delegation. A wrong authority source can expand access.
- Grant insertion and authority consumption must share the existing publication
  transaction; retention or retry must not recreate authority.
- HTML bootstrap insertion can break doctype, nested paths, encoding, or length
  headers. Stored artifact bytes must remain unchanged.
- A startup acknowledgement proves document/context startup, not application
  correctness. It must not become a privileged message bridge.
- Review source labels need current authorization and graceful deletion fallback.
- Concurrent branches may touch the shared canvas E2E fixture or spec pair;
  reconcile current base before implementation and final delivery.

---
created: 2026-11-15
status: in_progress
requirements:
  - REQ-PLUGINS-FILE-VIEWERS-001
  - REQ-PLUGINS-FILE-VIEWERS-002
  - REQ-PLUGINS-FILE-VIEWERS-003
  - REQ-PLUGINS-FILE-VIEWERS-004
  - REQ-PLUGINS-FILE-VIEWERS-005
  - REQ-PLUGINS-FILE-VIEWERS-006
system_design:
  - ../../specs/plugins/system-design/file-viewers.md
legacy_specs: []
---

# Implementation plan: File viewer plugins

## Overview

Deliver host-mediated read-only viewers, external-file permission, extension
marketplace discovery, and a separately packaged STL viewer. Execute sequentially.
The user accepted 64 MiB and one million triangles, native-plugin trust, and
external files with an Ask default. Implementation authorized by the user. All four work packages have implementation code. Focused local checks pass;
remote, Windows, and physical-device release gates remain open.

## Scope

Include every acceptance criterion in the paired requirements. Exclude model
editing, slicing, archives, remote model URLs, hostile-plugin sandboxing, and
Git-revision comparison. Production plugin source belongs outside this monorepo.

## Technical approach

Task 01 owns bounded file transport and external-policy enforcement. Task 02
adds the SDK/manifest/registry contract and connects host UI to it. Task 03 extends
catalog metadata and discovery. Task 04 builds and tests the standalone STL plugin.
Existing text reads retain their 10 MiB cap and external-read behavior.

The policy applies per user/workspace/installation/digest. Ask receipts are
single-use and generation-bound. Policies are host-owned, not plugin config.
Internal file opens and external approval use the same execution identity.

| Executor | Identity and transport | Required evidence | Failure fallback |
| --- | --- | --- | --- |
| Local/worktree | Selected execution root and authenticated agentctl | Rooted open, external receipt, cancellation, replacement | Deny preview; no alternative path |
| Docker | Container filesystem and agentctl transport | Same adversarial fixtures inside container | No backend-host file fallback |
| SSH | Remote user/path and existing authenticated tunnel | Identity binding and disconnect cancellation | Show unavailable; explicit retry |
| Kubernetes | Selected pod/runtime epoch and agentctl route | Pod replacement invalidates approval | Require new request |

No provider is certified by another provider's test. POSIX and Windows file
identity semantics require their own supported-runner evidence.

## UI composition previews

These labelled text descriptions are the structural previews; the task platform
uses rich output rather than ASCII drawings. Spacing and exact copy are illustrative.
Host controls, localization, focus behavior, safe areas, and permission ordering
are requirements. Desktop dialogs and phone drawers must preserve the same action.

UI-01: File preview, desktop and phone. Entry: opening a supported file.
Desktop uses the existing file panel with model viewport, dimensions strip,
Open with, Fit, and zoom. Phone uses a full-height file surface, readable
dimensions and bottom reachable controls. Workspace files load automatically;
errors offer Retry without automatic fallback. No GPU renders before permission.

UI-02: External approval and settings. Entry: explicit external-file open under
Ask. Desktop dialog: plugin identity, execution, resolved path, size, then
Preview once and Cancel. Phone drawer: scrollable wrapping metadata body and
fixed safe-area actions. Settings has a labelled Ask/Allow/Deny selector with
Ask selected initially and a concise trust explanation. Visible restored tabs
repeat admission. Only external files under Ask prompt. Cancel dismisses that
prompt; loading and ready previews stop through file close or navigation.

UI-03: Unsupported file and discovery. File panel displays format, Find viewer
plugin, and existing download/external-open. Browse displays an extension chip,
matching entries, and normal explicit Install actions. Phone uses the existing
full-page marketplace composition and a Back action to the file. Distinct
states: no match, offline catalog, incompatible, installed but disabled, and ready.

UI-04: STL ready. Model viewport, XYZ model-space dimensions, labelled grid,
Fit/Reset, zoom, and model-unit dimensions. Desktop supports pointer and keyboard;
phone uses full-height viewing, touch orbit/pinch/pan, and bottom controls.
Malformed input, resource exhaustion, and GPU failure show distinct recovery text.

## Tests and acceptance mapping

All test paths below are proposed new suites unless already present. Work orders
name exact commands. Use TDD for implementation. The implementation record names the delivered tests.

- 002.1-002.5 and 006.1-006.6: Go `TestFilePreview*` suites at agentctl process/API,
  agent runtime client, task handlers, and plugin policy boundary. Fuzz malformed
  paths and test approval replay, foreign scopes, replaced targets, mutation,
  limit+1 reads, disconnects, and concurrent admissions.
- 001.1-001.5, 003.4-003.5, 005.3-005.4, 006.1-006.7: SDK tests,
  `registry-file-viewers.test.ts`, `file-viewer-selection.test.ts`,
  `use-file-preview-policy.test.ts`, and file-viewer lifecycle tests.
- 004.1-004.5: manifest/index/catalog tests and
  the desktop/phone marketplace browser tests. Test exact extension matching, malformed
  declarations, package/listing mismatch, and return-to-file context.
- 003.1-003.5, 005.1-005.2, and 002.2: standalone STL parser, dimensions, camera,
  bounded-allocation, cancellation, and network-silence tests.

## E2E tests

Host fixture tests: `tests/plugins/file-viewer.spec.ts` (chromium) and
`tests/plugins/mobile-file-viewer.spec.ts` (mobile-chrome) cover selection,
restore, Ask/Allow/Deny, cancellation, revocation, and no bytes before consent.
Add `tests/plugins/file-viewer-marketplace.spec.ts` and
`tests/plugins/mobile-file-viewer-marketplace.spec.ts` for exact extension filters,
install/return, and empty/error states. Executor integration tests live in
`tests/docker/file-viewer-executors.spec.ts` in the containers project.
The plugin repo owns `e2e/stl-viewer.mjs` and `scripts/test-host.mjs`. The script
generates temporary desktop/phone specs against an isolated compatible Kandev instance.
The executor suite and the remaining lifecycle matrix are still required before release.

## Work orders

- [ ] [01: Bounded reads and external permission](task-01-bounded-file-access.md)
- [ ] [02: Viewer API and host interaction](task-02-viewer-api.md)
- [ ] [03: Extension discovery](task-03-marketplace-discovery.md)
- [ ] [04: Standalone STL viewer](task-04-stl-plugin.md)

## Verification results

See the [implementation record](../../reviews/file-viewers-implementation.md) for
completed checks, browser results, and release gates. Do not infer remote or
physical-device certification from local checks.

## Risks

Native code remains privileged; Ask does not isolate malicious plugins. Worker
memory overhead needs measurement. External files can change while permission is
pending. Each executor needs cancellation and file-identity evidence. The local production plugin repository is attached to this task. The SDK release
floor remains unresolved before publication; development packaging uses the
matching host checkout.

## Implementation prerequisites

Install dependencies once with `pnpm --dir apps install --frozen-lockfile` if absent.
Read scoped AGENTS.md, TDD, E2E, mobile-parity, and create-kandev-plugin guidance
before the relevant task. No task authorizes delegation, publication, or commits.
Do not mark a task done if its required environment or measurement is unavailable.

## Rich-preview integration delivered (2026-11-17)

AC-PLUGINS-FILE-VIEWERS-001.6 now uses the shared plugin viewer in rich output.
The implementation record documents the host changes, template recipe, and
77 focused tests plus desktop/phone packaged-STL checks. This extends Task 02
and Task 04 without a second viewer registration or rich-output schema change.

## Follow-up for supported files

[Task 03](task-03-marketplace-discovery.md#supported-file-regression) owns the
editor discovery bar correction. The correction passed focused component, desktop, and mobile tests.

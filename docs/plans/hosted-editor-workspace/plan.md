---
created: 2026-10-18
status: done
requirements:
  - REQ-WORKSPACES-HOSTED-EDITOR-001
system_design:
  - ../../specs/workspaces/system-design/hosted-editor-workspace.md
legacy_specs: []
---

# Implementation Plan: Hosted Editor Workspace

## Overview

Restore selected-workspace opening for custom hosted editors in one sequential
work order. Establish an independent real-service regression first, correct the
hosted dispatch/builder seam, then run only the owning package and changed-Go
lint plus documentation checks. ROOT accepted the design and issued the later explicit implementation request;
the work order is implemented and locally verified.

## Scope

In scope: resolved workspace-root targeting, including empty and lexically
root-equivalent targets; selected worktree/association identity and existing
fallbacks; ordinary URL path/query preservation; existing file, embedded, error,
and containment controls.

Out of scope: nested-folder API semantics, vendor-specific file navigation and
preselected URL precedence, remote path mapping, frontend/API changes, type
inference/stat, editor discovery/install/lifecycle, SSH/native policy, lexical
containment redesign, retries, pagination, and new settings or automation flows.

## Root cause and admission evidence

`Service.OpenEditor` resolves an empty file target to the nonempty selected root.
`dispatchEditorKind` loses the root/file distinction for `custom_hosted_url`;
`buildHostedURL` writes every nonempty path to `file`. The isolated helper's
`folder=/` case does not exercise a real root request.

ROOT's supplied qualification receipts under
`/tmp/kandev-root-hosted-editor-folder-3vvz68e4/` report one causal real-service
root failure and two passing controls: hosted file coordinates and embedded
root sentinel. Actual Go exit was 1; wrapper exit 0 was not a test pass. The
fixture was removed and ROOT reported a clean source tree. These are admission
evidence only; permanent tests must be independently authored, not copied or
replayed from ROOT's private source/runner. This design turn runs no Go proof.

## Technical approach

Pass the resolved root to the hosted URL construction boundary in
`apps/backend/internal/editors/service/service.go`. Compare cleaned root and
resolved target after admission; use `folder=<selected-root>` for equality and
retain existing `file` behavior for other targets. Preserve the base URL path
and ordinary non-target query values via `net/url`. Update existing helper
calls/tests only as required by the internal signature.

Consumer wiring is already complete: `EditorsMenu.openWith` omits `filePath`,
`useOpenSessionInEditor` sends existing optional fields, the controller invokes
the real service, and the hook opens the returned URL. Do not modify that path.
The owning [design](../../specs/workspaces/system-design/hosted-editor-workspace.md)
contains the integration compatibility matrix and error boundaries.

## Tests

Create `apps/backend/internal/editors/service/hosted_workspace_test.go` with
synthetic identities, temporary roots, and ordinary dependency fakes. The named tests below are implemented.

| Test / criteria | Independent regression and controls |
| --- | --- |
| `TestOpenEditor_HostedWorkspaceRoot` / AC 001.1, 001.2, 001.5 | Real service: omitted/empty target, `.`, `src/..`; worktree ID and association ID select the second of two roots; positive coordinates do not change folder value; no generated `file` on ordinary base URL. Must fail before correction. |
| `TestOpenEditor_HostedWorkspaceFallbacks` / AC 001.2 | Omitted selection uses first worktree; no worktrees uses single-repository local path; unknown explicit ID and empty selected path never fall back. |
| `TestOpenEditor_HostedCompatibility` / AC 001.3 | Real service contained file with no coordinates, line only, line+column, column without positive line; exact decoded legacy `file` value. Preserve ordinary base URL path/non-target repeated query values and fragment, root/file paths with spaces, `#`, plus, Unicode. No vendor file-navigation claim. |
| `TestOpenEditor_HostedWorkspaceErrors` / AC 001.4 | Empty session ID, missing session/workspace/editor, disabled editor, missing/invalid hosted configuration, malformed URL, dependency read error, parent and normalized escapes: exact existing error and empty URL. |
| Existing embedded/containment tests plus new embedded-root control / AC 001.4 | Exact `internal://vscode`, supported synthetic Docker capability without Docker startup; unsupported capability rejects. Existing `TestOpenEditor_ContainedDotPaths` and `TestResolveFilePath_ContainmentControls` remain intact. |

All abbreviated AC references in this table have prefix
`AC-WORKSPACES-HOSTED-EDITOR-`. Assert selected identities with independent
expected temporary paths and decoded URL values, never production predicates
or builders. No actual native editor, SSH handler, Docker, or hosted IDE launch.

## E2E evidence and mobile assessment

Real `Service.OpenEditor` to returned URL is the changed end-to-end operation
boundary. Desktop and phone share the opener; this backend data correction
changes no layout, touch, focus, scrolling, navigation, or copy. Apply the
mobile state/data exception with targeted service tests and source wiring
inspection. No frontend/browser build, Playwright gate, visual preview, or live
hosted rendering claim is warranted. Registered HTTP routing is not exercised.

## Public documentation assessment

`docs/public/developer-tools.md` already promises selected-session worktree
launching and hosted file/folder query values. No public copy or screenshot
change is needed; this restores that documented contract. The containment pair,
Tasks folder-action pair, and embedded capability ADR remain unchanged.

## Work orders

- [x] [Task 01: Open hosted workspace](task-01-open-hosted-workspace.md) (done; sequential)

## Verification results

Design checkpoint, 2026-10-18:

- `python3 scripts/list-docs.py validate`: passed (387 decisions, 1652 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog query for Workspaces `hosted-editor`: both new paired paths present.
- Work-order Node documentation-coverage preflight: `covered`, zero errors;
  validates intended changed-path coverage, not implementation execution.
- `git diff --check -- docs/specs docs/plans/hosted-editor-workspace`: passed.
- `git status --short`: only the four new design-package files, unstaged and
  uncommitted; no production or permanent test modifications.

Implementation verification, 2026-10-18:

- Independently authored real-service RED: Go exit 1, six root cases failed
  specifically on `file` versus `folder`; all four hosted file coordinate cases,
  embedded selected-root control, and repository-less embedded control passed.
  Owning package execution: 0.031s.
- Final owning service race command from the work order: passed, 1.060s.
  This includes root selection/fallbacks, hosted compatibility/errors, embedded
  capability controls, and unchanged containment tests.
- Canonical `bash scripts/lint-go-changed` on the three actual changed Go paths:
  passed, zero issues. The initial run required changing the new if/else chain
  to a switch; the final production revision was race-tested again.
- Catalog validation, 36 specification-linter tests, all specification lint,
  intended-path documentation coverage, and diff whitespace checks passed.
- No native editor, SSH handler, Docker, browser, or hosted IDE launched.
  Registered HTTP routing and live IDE rendering were not exercised.

Normal publication hooks and exact-head hosted CI/review evidence are recorded
in the external task plan/conversation after commit; they are separate from
these local results. No broad local confidence suite was run.

## Risks

- Comparing only raw input emptiness would miss dot/normalized roots; comparing
  only target to an empty string repeats the defect.
- Selecting the first worktree instead of the requested identity can yield a
  plausible but wrong folder URL; use distinct roots in real-service tests.
- Query encoding and coordinate suffixes must preserve existing file behavior.
- Hosted services still need access to backend-host absolute paths; this change
  does not map remote paths or resolve configured vendor preselection conflicts.
- Unit/service URL proof does not establish external IDE rendering or vendor
  support for the retained `file` parameter.

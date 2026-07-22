---
spec: docs/specs/ui/external-vcs-file-links.md
created: 2026-07-22
status: complete
---

# Implementation Plan: External VCS File Links

## Overview

Add one shared, fail-closed external-file URL resolver and one provider-aware link control, hydrate the linked-review metadata once per task, then reuse the control across all existing diff/file toolbar families. The backend exposes the already-persisted repository `remote_url` through its shared read-only DTO; generic repository settings must not gain a new clone-URL write path. Linked GitHub PR, GitLab MR, and Azure DevOps PR branches already have frontend stores and loaders.

## Backend

`task.Repository` already persists `remote_url`, but the shared repository DTO did not emit it. Add read-only DTO serialization and focused regression coverage. Do not accept `remote_url` through generic repository create/update settings because the same field is a clone target; repository creation continues to populate it through the existing provider/task resolution path. GitHub `TaskPR.head_branch`, GitLab `TaskMR.head_branch`, and Azure DevOps `TaskPR.sourceBranch` already expose published review branches.

## Frontend

### External URL contract and task metadata

- `apps/web/lib/types/http.ts`: add the existing backend `remote_url` field to the `Repository` TypeScript contract.
- `apps/web/lib/utils/external-vcs-file-url.ts`: add a pure `resolveExternalVcsFileURL` helper. It validates credential-free HTTPS origins, safely derives canonical web origins from supported HTTPS or SSH clone identities, produces GitHub `blob`, GitLab `-/blob`, and Azure DevOps `path`/`version=GB...` URLs, and applies added/deleted/renamed revision/path rules from the spec.
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.ts`: resolve the file's task repository from explicit repository ID first, repository name second, and the session's sole repository only when unambiguous. Match published branches by provider + repository and, for repeated-repository/multi-branch tasks, the active worktree branch; otherwise use the exact task/session base branch. Return `null` for ambiguous or incomplete contexts.
- The same hook exposes a task-level hydration helper used once by `apps/web/components/task/task-page-content.tsx`. It enables only the applicable existing loaders: `useTaskPR`, `useWorkspaceMRs`, and `useAzureDevOpsTaskPullRequests`, so per-file controls read stores without issuing duplicate requests.
- `apps/web/components/editors/external-vcs-file-link.tsx`: render a provider-brand semantic anchor styled as an icon button, with `target="_blank"`, `rel="noopener noreferrer"`, and provider-specific accessible name/tooltip. Support compact desktop (`xs`/`sm`) and 44px touch sizing.

### Toolbar integrations

- `apps/web/components/diff/{file-diff-viewer.tsx,diff-viewer.tsx,use-diff-options.tsx,diff-header-toolbar.tsx}`: carry session/repository/status/previous-path context into the common Pierre diff header and place the shared action beside existing file actions. This covers Changes and commit-detail diff headers on desktop and in the mobile diff drawer.
- `apps/web/components/review/{review-diff-list.tsx,review-diff-toolbar.tsx}`: pass the `ReviewFile` source/status/old-path/repository identity into the sticky Review toolbar and render the same action. Explicit PR-source context wins over inferred local state.
- Built-in editor/viewer toolbars: render the action beside existing file actions for Monaco and CodeMirror text/Markdown editors, Monaco and Pierre diff headers, and desktop image/binary viewers.
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`: render the touch-sized action in the dedicated mobile file-viewer header for text, Markdown, image, and binary viewers.

### Mobile design contract

- **Desktop outcome:** a compact provider icon in each existing diff/file toolbar opens the externally hosted file without leaving Kandev.
- **Mobile entry point:** the existing `MobileFileViewerPanel` header and Review sticky file header expose the same action directly; the full-height `MobileDiffSheet` inherits it through the shared diff toolbar.
- **Nearest exemplars:** `mobile-file-viewer-panel.tsx` supplies the dedicated dense-file header; `mobile-review-file-status.spec.ts` supplies the sticky Review header and no-horizontal-overflow contract.
- **Hierarchy/presentation:** this is one immediate secondary action, so it stays inline rather than opening a drawer or menu. File content remains the focal surface.
- **Geometry:** existing CodeMirror/Pierre content remains the single scroll owner. The new mobile control has a 44px active dimension; no fixed positioning or new safe-area owner is introduced.
- **Shared logic:** URL, repository, revision, and status resolution are identical across viewports; only the control size changes.

## Tests

- **Provider URL construction and security:** `apps/web/lib/utils/external-vcs-file-url.test.ts` uses table-driven cases for GitHub, self-hosted GitLab, Azure DevOps, encoded refs/paths, clone suffix removal, non-HTTPS/credential rejection, and unsupported providers.
- **Revision/path selection:** the same utility tests cover published-head preference, base fallback, added/untracked suppression, deleted-base targeting, and renamed old/new path behavior.
- **Repository and branch resolution:** `apps/web/hooks/domains/workspace/use-external-vcs-file-link.test.tsx` covers single repo, multi-repo, repeated-repository/multi-branch matching, legacy unambiguous fallback, and fail-closed ambiguity.
- **Semantic action:** `apps/web/components/editors/external-vcs-file-link.test.tsx` verifies provider label/icon, new-tab/rel attributes, compact/touch sizing, and omission when the hook returns no URL.
- **Diff toolbar wiring:** extend `apps/web/components/diff/diff-header-toolbar.test.tsx` and add/extend `apps/web/components/review/review-diff-toolbar.test.tsx` to prove file/repository/status context reaches the shared action without regressing existing controls.
- **Editor/mobile wiring:** add focused component coverage for Monaco and CodeMirror editors/diffs, desktop image/binary viewers, and `mobile-file-viewer-panel.tsx` for link presence and omission.

Focused backend DTO coverage proves persisted `remote_url` is serialized without introducing a generic write path. Rendered component tests plus real browser E2E cover the complete existing-state-to-anchor path.

## E2E Tests

- **Desktop published branch:** `apps/web/e2e/tests/review/external-vcs-file-link.spec.ts` configures a GitHub repository and linked PR, opens Review/Changes, activates `Open file in GitHub`, and asserts the new tab targets the PR head file URL.
- **Desktop base fallback and unavailable added file:** the same spec verifies an existing file links to base when no PR is linked and an added/untracked file does not offer the action.
- **Mobile parity:** `apps/web/e2e/tests/task/mobile-external-vcs-file-link.spec.ts` opens a supported file through Files, verifies the 44px provider action and target URL, activates it in a new tab, and asserts no document-level horizontal overflow.

## Implementation Waves

Wave 1:

- [x] [task-01-link-foundation](task-01-link-foundation.md) — done; implementer, balanced model

Wave 2:

- [x] [task-02-toolbar-wiring](task-02-toolbar-wiring.md) — implementer, balanced model; depends on task 01 — done

Wave 3:

- [x] [task-04-repository-remote-url-contract](task-04-repository-remote-url-contract.md) — implementer, balanced model; unblocked browser coverage after the real HTTP contract omitted `remote_url` — done
- [x] [task-03-browser-coverage](task-03-browser-coverage.md) — test-engineer, balanced model; depends on tasks 01-02 and task 04 — done

After Wave 3, run delegated simplification, security review, QA, and full verification in that order.

Review remediation:

- [x] [task-05-safe-remote-url-exposure](task-05-safe-remote-url-exposure.md) — removed the unsafe generic write-through, retained read-only DTO exposure, and seeded E2E through the existing provider/task flow — done
- [x] [task-06-resolver-correctness](task-06-resolver-correctness.md) — fixed single/repeated repository resolution and added safe SSH clone identities — done
- [x] [task-07-toolbar-completeness](task-07-toolbar-completeness.md) — completed CodeMirror, Monaco diff, image/binary toolbar coverage and restored the TS file-size limit — done

## Verification

Targeted commands are recorded in each task file. Final verification runs from the repository root in the required order:

```bash
make fmt
make typecheck test lint
```

Focused browser verification runs the two new Playwright specs against rebuilt production assets:

```bash
cd apps/web && pnpm e2e -- tests/review/external-vcs-file-link.spec.ts --project=chromium
cd apps/web && pnpm e2e -- tests/task/mobile-external-vcs-file-link.spec.ts --project=mobile-chrome
```

## Risks

- Repeated-repository tasks can have several linked reviews for one repository ID. Resolution must include active branch context and fail closed when no unique review branch can be selected.
- Provider clone URLs may contain `.git`, nested GitLab namespaces, ports, or credentials. The resolver must preserve valid host/path identity while rejecting any URL that could expose credentials.
- Dense Pierre/Review headers can overflow on narrow viewports. The mobile test must prove action reachability and unchanged scroll ownership.

---
created: 2026-11-05
status: in_progress
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/tasks/system-design/chat-file-link-context.md
  - ../../specs/platform/system-design/git-repository-scope-lifecycle.md
legacy_specs: []
---

# Live repository attachment fixes

## Overview

This package repairs [#5070](https://github.com/kdlbs/kandev/issues/5070) and [#5071](https://github.com/kdlbs/kandev/issues/5071).
Both defects follow a live legacy add-branch call that promotes Files to the parent task folder.
The agent remains in its original checkout.

Implement relative-link context first, then Git scope reconciliation, then the combined browser regression.
All work orders are complete and were implemented sequentially. This package authorizes no implementation delegation.
The implementation added production fixes, permanent regressions, and managed browser proof.

## Evidence and root causes

Investigation used checkout `360b05ccd378cbe8c08bfcdc86a9c9a9d3465fd7` on 2026-11-05.
Both canonical issues were open, with no comments or image attachments.
Both now belong to the authenticated GitHub user `carlosflorencio`.
The issue reports contain live-instance evidence from revision `a8b8ac18e0f08cafc48f303eafc88ffd546f31f4`.
This investigation did not access that reporter's database or live instance.

A temporary Vitest file exercised the real resolver, Git event handler, runtime store, and Changes derivation.
Command: `(cd apps/web && pnpm exec vitest run lib/markdown/live-repository-attachment.repro.test.ts)`.
All three assertions failed for the expected behavior differences:

| Reproduction | Expected | Observed |
| --- | --- | --- |
| #5070: relative link after root promotion | `master/implementation_plans/node26-runtime-upgrade/task-00-debian-base.md` | Missing `master/` |
| #5071: unnamed failure, then two ready named scopes | No failed repository | `failedRepositories: [""]` |
| #5071: same sequence with legacy cleanup | No failed repository | `failedRepositories: [""]` |

The temporary file was removed after diagnosis.
These failures establish defects in the current source, not a completed fix.
No fresh-instance browser reproduction ran during this design turn.

For #5070, `resolveMarkdownFileTarget` returns relative hrefs unchanged.
`TaskMarkdownFileLinkProvider` supplies the current Files root and absolute aliases, without a relative checkout base.
`handleFileContent` therefore reads from the promoted parent folder.

For #5071, `clearLegacyGitStatusEntry` removes old snapshots but leaves refresh failures.
`SetWorkspacePollMode` and `refreshTrackersDetached` also refresh the file-only parent tracker.
Named successes update their own keys and cannot clear the obsolete empty key.
`failedRepositoryNames` then produces a blank-name sentence.

## Requirements and design

Tasks owns checkout continuity after attachment.
Use [REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001](../../specs/tasks/requirements/attach-workspace-sources.md), criteria `.5` through `.8`.
Criteria `.7` and `.8` make the missing relative-link behavior explicit.
The [chat context design](../../specs/tasks/system-design/chat-file-link-context.md) defines the three distinct roots.

Platform owns repository refresh health.
Use [REQ-PLATFORM-WORKSPACE-GIT-STATUS-001](../../specs/platform/requirements/workspace-git-status.md), criteria `.25`, `.26`, `.27`, `.39`, `.40`, and `.41`.
Criteria `.26` and `.41` clarify current scope membership without creating a separate repair requirement.
The [scope lifecycle design](../../specs/platform/system-design/git-repository-scope-lifecycle.md) defines retirement and recovery.

The [accepted ADR](../../decisions/2026-11-05-chat-file-link-workspace-context.md) records chat-base authority.
Existing accepted decisions retain live add-branch continuity and environment-owned Git state.

## Scope

In scope:

- Preserve relative links through unchanged-CWD root promotion and reload.
- Retain explicit source prefixes, absolute aliases, containment, and duplicate-filename isolation.
- Stop Git refresh publication from a plain parent tracker.
- Retire obsolete refresh state without hiding failures in current repositories.
- Cover active-turn legacy attachment on desktop and phone.

Out of scope:

- Moving or restarting the agent, terminals, or workspace processes.
- Changing attachment placement, provider sandbox access, or branch materialization.
- General historical per-message CWD capture or arbitrary shell `cd` tracking.
- Changing repositoryless initial Git status or explicit invalid-repository queries.
- New polling systems, feature flags, metrics, database migrations, or broad cache resets.

## Technical approach

Task 01 projects `agent_workspace_path` from the live execution and durable current harness generation.
Session hydration and ready events preserve it separately from `workspace_path` and `worktree_path`.
The transcript provider passes it through shared and memoized Markdown context.
The resolver preserves explicit source prefixes before it applies the relative base.

Task 02 uses real tracker eligibility for explicit Git refreshes.
It uses a validated correlated aggregate response to reconcile runtime scope membership.
One response must contain one source's complete scope set, including unavailable repository slots.
A local environment generation fences requests across attachment and reconnect.
The shared coordinator remains the only retry owner.

### Compatibility matrix

| Runtime shape | Relative-link authority | Git health | Evidence |
| --- | --- | --- | --- |
| Worktree, live legacy attachment | Unchanged native workspace | Named scopes, plain parent excluded | Both issue reproductions and targeted browser flow |
| Session launched at parent root | Confirmed parent workspace | Actual registered scopes | Unit and browser counterexample |
| Git root plus nested repository/submodule | Confirmed native workspace | Valid empty scope plus named children | Tracker/store mixed-state regression |
| Remote executor or shared environment | Same typed roots and current environment | Existing authenticated source selection | Shared-path unit coverage. No claim of a remote browser run |
| Legacy payload without base evidence | Existing single-root behavior | Existing status behavior | Unit compatibility cases |
| Ambiguous expanded-workspace base | Block unqualified link | Preserve current failures | Fail-closed resolver and state cases |

## ASCII UI preview

`UI-01: Chat file opening`. Entry: a transcript file link after attachment.
The before view follows source and temporary-test evidence.

```text
Before: Chat [base-image work order] -> Failed to open file

Desktop after:
+----------------------------+----------------------------------+
| Chat                       | File panel                       |
| [base-image work order]     | master/implementation_plans/...   |
|                            | Exact primary-checkout contents  |
+----------------------------+----------------------------------+

Phone after:
+----------------------------+
| Chat                       |
| [base-image work order]     |
+----------------------------+
              tap
               v
+----------------------------+
| Back   task-00-...md  Close | fixed existing controls
| Exact primary-checkout     |
| contents                   | existing file scroller
+----------------------------+
```

`UI-02: Changes refresh health`. Entry: keep Changes active during attachment.

```text
Before: Changes [warning] -> " could not be refreshed."

Desktop after: Changes       Phone after: Changes tab
+-----------------------+   +-----------------------+
| MASTER                |   | MASTER                |
|   changed-file.md     |   |   changed-file.md     |
| SECONDARY             |   | SECONDARY             |
|   clean               |   |   clean               |
+-----------------------+   +-----------------------+
  No refresh warning          Existing vertical list

Current named failure: Changes [warning] -> Secondary could not be refreshed.
Current Git-root failure: Changes [warning] -> Existing generic retry notice.
```

The structure is required. Filenames, counts, spacing, and translated labels are illustrative.
The existing desktop panels and phone full-height viewer remain the composition exemplars.
Phone uses Chat and Changes entry points, one content scroller, existing safe areas, and reachable Back/close controls.
Dense file content needs the existing full-height viewer rather than another picker.
No new touch or hover interaction is necessary. No new locale keys are expected.
These previews map to Tasks `.7/.8` and Platform `.25/.26/.41`.

## Tests

| Acceptance | Permanent regression location and case |
| --- | --- |
| Tasks `.5/.6/.7` | `session_agent_workspace_test.go`: `TestTaskSessionAgentWorkspaceProjection`; `agent_workspace_projection_test.go` and ready-event tests retain distinct roots |
| Tasks `.7/.8` | `file-link-target.test.ts`: unqualified links after promotion, explicit prefixes, duplicate names, parent-root base, missing/ambiguous base, containment |
| Tasks `.5/.7` | Provider, Markdown, memoized context, session merge, and ready-handler tests exercise both hydration orders |
| Platform `.25/.26` | `manager_attachment_refresh_scopes_test.go`: `TestManager_LiveRepositoryAttachmentRefreshScopes` exercises plain parent and Git-root graphs |
| Platform `.26/.41` | `git-status.test.ts`: retire unnamed failure and preserve a failed current sibling or root |
| Platform `.27/.40` | Refresh coordinator and hook tests reject old-generation responses, share sibling work, and refresh after attachment |
| Platform `.39/.41` | Coordinator and Changes tests stop recovery only after all current failures clear |

Each work order started with its behavioral regression under `/tdd`.
The regression files and test names above are the implemented targets.

## E2E tests

Add `tests/task/live-repository-attachment.spec.ts` for Chromium.
Add `tests/task/mobile-live-repository-attachment.spec.ts` for `mobile-chrome`.
Use a shared helper and a mock-agent scenario that invokes the real legacy MCP tool during its turn.
Do not substitute the idle batch attachment path or direct store writes for this integration proof.

Both flows preserve a link emitted before attachment and open links emitted afterward.
Both prove exact file content with duplicate filenames in the two repositories.
Keep Changes mounted across attachment, retain primary changes, and wait for real named snapshots.
Repeat after reconnect/reload. Include a parent-root session as a relative-base counterexample.
Unit and tracker tests own partial failure, late frames, and valid unnamed-root combinations.
Use causal HTTP/WS waits. Managed runs build fresh binaries and Vite assets.

## Work orders

- [x] [Task 01: Preserve relative chat file context](task-01-preserve-chat-file-context.md)
- [x] [Task 02: Reconcile Git repository refresh scopes](task-02-reconcile-git-refresh-scopes.md)
- [ ] [Task 03: Prove live attachment on desktop and phone](task-03-prove-live-attachment.md)

## Companion packages

The completed [chat root repair](../multi-repo-chat-file-links/plan.md) covers absolute links after reload.
The completed [source alias package](../repository-source-file-links/plan.md) covers registered absolute source aliases.
The [Changes refresh package](../changes-panel-git-refresh/plan.md) and [recovery package](../changes-loading-feedback/plan.md) own the existing refresh protocol.
This package extends their missing transition cases. Their historical results remain unchanged.
Any implementation change to their owned behavior must rerun the affected focused tests listed here.

## Verification results

Implementation and browser proof are complete on 2026-11-05.
Both work orders passed their focused regressions; the Chromium and
`mobile-chrome` specs passed against fresh managed builds.
The phone proof uses Playwright `mobile-chrome` emulation; a physical-device
smoke check of the mobile Chat and Changes flows was not available in this
environment and remains a release gate before closing the issues.

Evidence:

- Session/DTO/lifecycle projection and ready-event tests: passed.
- Resolver, provider, Markdown context, session merge, and WS tests: passed.
- The real-`StateProvider` fallback-link regression rejects an unstable
  external-store snapshot and passes through unrelated store updates.
- Agentctl plain-parent/Git-root disclosure and backend aggregate tests: passed.
- Inventory reconciliation, refresh coordinator, Git-status handler, and
  Changes panel tests: passed.
- `(cd apps/web && pnpm e2e:run --host --project chromium tests/task/live-repository-attachment.spec.ts)`:
  1 passed.
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-live-repository-attachment.spec.ts)`:
  1 passed.
- `pnpm run typecheck`, changed-file Oxlint, and `pnpm run i18n:ratchet`: passed.
- `make -C apps/backend build`: passed.
- Specification/documentation checks rerun during delivery.

Design-package checks passed:

- `python3 scripts/list-docs.py validate`: 432 decisions and 1,922 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Local `.github/scripts/pr-docs.cjs` coverage preflight: all three work orders covered, with no reference errors.
  The design preflight includes the planned resolver change because this turn changes documentation only.
- `git diff --check -- docs/plans/live-repository-attachment docs/specs docs/decisions`: passed.

All package files and implementation changes remain unstaged and uncommitted for review.

## Risks and documentation impact

The primary checkout and native workspace can differ from launch time onward.
Do not replace native workspace evidence with repository count or a primary-path guess.
The new nullable session field needs correct omission, null, and hydration behavior.
The SQL projection must retain scan order and SQLite/PostgreSQL portability.

One ready repository cannot prove complete inventory or recovery of another repository.
A named child cannot prove retirement of a valid unnamed Git root.
Inventory generation must fence failures and cancellation bookkeeping as well as successful reads.

Public documentation needs a short explanation of the session-detail field and relative link behavior.
Task 01 owns updates to `docs/public/websocket-api.md` (reference) and `docs/public/automation-and-mcp.md` (reference).
No new UI label, command, configuration key, or screenshot is planned.
The production fix and permanent regressions are implemented. No commit or PR exists at this checkpoint.

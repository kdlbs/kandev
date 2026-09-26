---
created: 2026-09-23
status: complete
legacy_specs: []
requirements:
  - REQ-TASKS-CONVERSATION-FORK-001
  - REQ-TASKS-CONVERSATION-FORK-002
  - REQ-TASKS-CONVERSATION-FORK-003
  - REQ-TASKS-CONVERSATION-FORK-004
  - REQ-TASKS-CONVERSATION-FORK-005
  - REQ-TASKS-CONVERSATION-FORK-006
system_design:
  - ../../specs/tasks/system-design/conversation-forks.md
---

# Implementation plan: Conversation forks

## Overview

Create a portable conversation snapshot before destination launch.
Build the snapshot service, attachment copies, and destination admission first.
Then deliver the shared preview and new-agent flow, followed by task and child-task entry points.
Each work order owns its targeted tests. All five work orders are complete.

## Planning basis

The user requested this package after the conversation-fork proposal.
The package includes three destinations, a context chip, pre-launch estimates, and immutable snapshots.
Tasks own the feature because they own conversation records, destination relationships, and initial-prompt admission.

The implementation uses a 24-hour expiry, 4 MiB text bound, 10,000-row bound, and 20-active-draft quota.
Context-window estimates do not gate launch.
The interview confirmed optional tool inclusion through one switch and explicit attachment selection.
New agents share execution files. New tasks require separate execution files. Child tasks offer both choices.
The interview also confirmed existing task defaults for the code baseline of a separate workspace.
Token estimates remain informational. Oversized context can reach the provider and use existing failure handling.
No summarization, context-limit launch gate, or special oversized-context recovery belongs to the first version.
The planning interview is complete, and implementation follows its confirmed decisions.

## Scope

### In scope

- One ordinary task session, with an inclusive message cutoff and optional later range start.
- Text, completed clarifications, one optional tool-evidence switch, and explicit attachment copies.
- Immutable draft snapshots, provenance, expiry, admission receipts, and destination retention.
- Estimates for the selected model with explicit unknown limits and attachment overhead.
- New task, child task, and new agent destinations within the source Kandev workspace.
- Fresh conversation delivery, create-without-start, deferral, retry, and reload behavior.
- Desktop, keyboard, phone, and six-language interface support.

### Out of scope

- Provider-native forks, filesystem time travel, and cross-workspace transfer.
- Office, Quick Chat, terminal-only source history, and new agent-facing MCP tools.
- Automatic AI summaries, arbitrary transcript edits, and new runtime feature flags.
- Changes to normal history storage, plugin event reconciliation, or workspace reuse rules.

## Sources and companion packages

- [Requirements](../../specs/tasks/requirements/conversation-forks.md)
- [System design](../../specs/tasks/system-design/conversation-forks.md)
- [Source reconciliation decision](../../decisions/2026-09-16-conversation-source-reconciliation.md)
- [Saved-prompt authority decision](../../decisions/2026-09-01-server-owned-saved-prompt-expansion.md)
- [Attachment implementation record](../prompt-attachments/plan.md)
- [Saved-prompt launch repair](../saved-prompt-launch-fallback/plan.md)
- [Initial task brief](../initial-task-brief/plan.md)

These companion packages retain their existing scopes and results.
This package extends their interfaces without claiming their pending work is complete.
During implementation, existing source behavior was reconciled at the named integration points.
No existing companion plan owns the conversation-fork outcome, so none needs a duplicate work order.

## Technical approach

1. Add a task-owned compiler and repository snapshot read. Read authorized rows in one short transaction.
   Add immutable storage and authenticated draft, content, candidate, estimate, and discard endpoints.
   Keep current-state source reads unchanged. Use the existing tokenizer dependency with an explicitly approximate fallback.
2. Extend attachment staging with draft-owned physical copies. Reuse existing limits, private storage, and cleanup.
   Never repurpose source claims. Stage all selected copies before declaring the snapshot ready.
3. Add optional fork references to existing task creation and session launch DTOs.
   Create the destination, bind context and attachment claims, and save the creation receipt atomically.
   Carry admitted references through first-prompt ownership, workflow starts, deferral, and recovery.
4. Add a shared domain hook, typed API client, removable chip, and responsive preview.
   Integrate MessageActions and NewSessionDialog. Reuse the saved-prompt chip interaction without saved-prompt expansion semantics.
5. Integrate TaskCreateDialog and NewSubtaskDialog with the same snapshot state.
   Preserve existing repository/workflow defaults and create-without-start behavior. Add public guidance after delivery works.

The work orders record implemented files and verification results. Existing symbols are identified in the system design.

## ASCII UI preview

### UI-01: Message action and destination choice

Entry: a finalized message in an ordinary task session. State: draft compilation starts.

```text
Desktop message actions
[Copy] [Raw] [Info] [Star] [Fork from here]

+ Fork conversation ---------------------------+
| Through: Assistant message, 14:32             |
| ( ) New task                                 |
| ( ) Child task                               |
| ( ) New agent on this task                    |
| Preparing conversation...                    |
|                         [Cancel] [Continue]   |
+----------------------------------------------+

Phone: visible message overflow [More]
+ Destination picker (inset bottom drawer) -----+
| Fork conversation                            |
| New task                                  >  |
| Child task                                >  |
| New agent on this task                    >  |
+----------------------------------------------+
```

Continue opens the existing destination form. Preparation does not start an agent.
The phone picker is short and temporary. Each row has a touch target of at least 44 pixels.

### UI-02: Creation with a conversation chip

Entry: destination selection. State: ready snapshot, selected model, new instruction.
Numbers and source labels are illustrative.

```text
Desktop: existing task, child-task, or new-agent dialog
+ New agent on this task ---------------------------------+
| [Conversation: 42 messages | ~12.4K tokens | Preview | x] |
| ~6.2% of selected 200K context. Conversation only.       |
| [Write the next instruction...                         ]|
| [Agent/profile] [Model] [Executor]                      |
| Workspace: current task files                          |
|                                  [Cancel] [Start agent]|
+--------------------------------------------------------+

Phone: dedicated full-height creation surface
+------------------------------------------------+
| [Back] New agent                               | fixed
| [Conversation: 42 messages | ~12.4K | View | x] |
| ~6.2% of context. Additional overhead applies.  |
|                                                |
| [Write the next instruction...                ]| scroll
| [Agent/profile                              >] |
| [Model                                      >] |
| [Executor                                   >] |
| Workspace: current task files                   |
|------------------------------------------------|
| [Start agent                                 ] | fixed
+------------------------------------------------+
```

Task variants keep the existing repository, workflow, executor, and start-mode controls.
A child form shows its parent. A new task has no parent.
New-agent forms state Shared with current task. New-task forms state Separate workspace.
Child-task forms show [Share parent workspace | Separate workspace].
These rules apply on desktop and phone. No flow restores historical files.

### UI-03: Full preview and selection

Entry: chip click, keyboard activation, or phone tap. State: preview with optional selection changes.

```text
Desktop: preview subview inside the creation flow
+ Conversation preview ----------------------------------+
| [Back to creation] Source task / session                | fixed
| From [Session start v] Through [selected message]       |
| [Conversation] [Tool evidence] [Attachments]             |
|--------------------------------------------------------|
| User: ...                                              |
| Assistant: ...                                         | scroll
| ... complete compiled text ...                         |
|--------------------------------------------------------|
| Excluded: tools 12, reasoning 3, attachments 2           |
| ~12.4K tokens (approximate tokenizer)   [Apply selection]|
+--------------------------------------------------------+

Phone: full-height preview replaces creation view
+------------------------------------------------+
| [Back] Conversation preview                     | fixed
| Source task / session                           |
| Range [Session start through selected message >]|
| [Conversation v]                                |
|------------------------------------------------|
| User: ...                                      |
| Assistant: ...                                 | scroll
| ...                                            |
|------------------------------------------------|
| ~12.4K tokens. Attachments unmeasured.           |
| [Apply selection                             ] | fixed
+------------------------------------------------+
```

Phone sections use one section picker rather than horizontal tabs.
The evidence section has one Include tool output switch, off by default, plus a read-only inventory.
The attachment section has individual checkboxes, availability, and size.
Long code blocks contain their own horizontal overflow without widening the page.
Back and Escape leave the preview before dismissing creation. Focus returns to the chip.

### UI-04: Loading and recoverable errors

```text
[Conversation | Preparing...]         [Start disabled]
[Conversation | Expired] [Rebuild] [x] [Start disabled]
Tool output is unavailable. [Exclude tools] [Retry]
Estimated context: ~220K / 200K (110%). [Start available]
Estimate unavailable for this model. Context limit unknown.
```

Errors retain the instruction and current selection. Unknown estimates do not invent a percentage.
Context percentages remain informational even above 100%. No context-limit warning or launch block is added.
Provider rejection uses the existing launch or prompt error surface.

The structure, control order, accessible outcomes, and scroll ownership are requirements.
Spacing, icons, and sample numbers are illustrative. Use existing primitives and tokens.
UI-01 maps to AC-001.1 and AC-006.1. UI-02 maps to AC-003.1 and AC-004.1 through AC-004.2.
UI-03 maps to AC-001.5, AC-002.3 through AC-002.5, and AC-003.2.
UI-04 maps to AC-003.4 through AC-003.6 and AC-006.4.
All abbreviated AC references in this preview use the `AC-TASKS-CONVERSATION-FORK` prefix.
UI-02 and UI-03 also require AC-006.2, AC-006.3, and AC-006.5.

## Tests

All named new suites and methods below are implementation targets, not current passing evidence.
Use TDD for changed logic. Record RED, GREEN, and final command results in the owning work order.
All AC shorthand in this table has the `AC-TASKS-CONVERSATION-FORK` prefix.

| AC references | Test file and required method/scenario | Work order |
| --- | --- | --- |
| 001.2, 001.3, 001.4, 001.5 | `internal/task/repository/sqlite/conversation_fork_test.go`: `TestConversationForkSnapshot` (tie order, concurrent updates, cutoff, limits) | 01 |
| 002.1, 002.2, 002.3, 002.5, 002.6 | `internal/task/service/conversation_fork_test.go`: `TestConversationForkCompile` (clarification cutoff, filtering, hostile delimiters, unavailable evidence, nested fork) | 01 |
| 003.3, 003.4, 003.5, 003.6 | `internal/task/service/service_conversation_fork_test.go`: `TestConversationForkEstimateUsesKnownSelectedModelContextLimit` and draft estimate tests (known/unknown limits, informational estimates) | 01 |
| 005.1, 005.2, 005.3 | `internal/task/handlers/conversation_fork_handlers_test.go`: `TestConversationForkDraftAPI` and repository `TestConversationForkRetention` | 01 |
| 002.4, 002.5, 005.4 | `internal/task/service/conversation_fork_attachments_test.go`: `TestConversationForkAttachments` (copy isolation, unavailable bytes, aggregate bounds, cleanup) | 02 |
| 004.1, 004.2, 004.5, 005.1, 005.4 | `internal/task/service/conversation_fork_admission_test.go`: `TestConversationForkAdmission` (three destinations, rollback, duplicate race, revoked access) | 03 |
| 004.3, 004.4, 004.7, 004.8, 002.6 | `internal/orchestrator/conversation_fork_launch_test.go`: `TestConversationForkLaunch` (workflow templates, deferral, restart, literal historical mentions, source unchanged) | 03 |
| 002.4, 004.3 | `internal/orchestrator/conversation_fork_attachments_test.go`: `TestAppendConversationForkAttachmentsChecksCombinedLimits`; `internal/orchestrator/conversation_fork_launch_test.go`: `TestLaunchSessionDeliversAgentForkAttachments`, `TestStartTaskDeliversForkAttachmentsWithNewPromptAttachments`, and `TestStartCreatedSessionDeliversPendingForkAttachments` (copied files reach agent, new-task, and delayed-task launch requests) | 03 |
| 004.4 | `internal/task/repository/sqlite/conversation_fork_admission_test.go`: `TestConversationForkPendingTaskBindsFirstSessionAtomically` (first claim only, including ordinary and workflow-created later sessions) | 03 |
| 004.8 | `internal/task/service/conversation_fork_admission_test.go`: `TestConversationForkSeparateWorkspaceRejectsLocalExecutor` (new task and separate child fail before creation) | 03 |
| 004.6, 005.3 | Repository `TestConversationForkDestinationRetention` (reload, source deletion, destination cleanup) | 03 |
| 001.1, 003.1, 003.2, 003.4, 006.4 | `components/task/conversation-fork-flow.test.tsx` and `hooks/domains/task/use-conversation-fork.test.ts`: eligibility, generation races, preview navigation, retry | 04 |
| 001.1, 001.4 | `hooks/domains/task/use-conversation-fork.test.ts`: `creates snapshots from an accepted user cutoff while its turn is active` | 04 |
| 004.1, 004.2, 004.4 | `components/task/conversation-fork-destinations.test.tsx`: correct parent, source session, pending start, independent form defaults | 05 |

Repository tests must cover both SQLite and PostgreSQL using `testutil.OpenIsolatedPostgres`.
PostgreSQL cases live in `conversation_fork_postgres_test.go` with `TestConversationForkPostgres` names.
A skipped PostgreSQL run is not passing evidence. Each relevant work order requires a configured test database.

## E2E tests

Browser files are under `apps/web/e2e/tests/session/` and share `conversation-fork-helpers.ts`.
Use `../../fixtures/test-base` and seed through API helpers. Assert the user outcome through the UI.
Extend the isolated mock agent only when necessary to inspect the delivered first prompt.
Do not start real provider sessions. Read the assigned preview before implementation and compare the rendered result.

| Scenario | Desktop file / project | Phone file / project | AC references |
| --- | --- | --- | --- |
| `new agent receives frozen history through selected message` | `conversation-fork-agent.spec.ts` / chromium | `mobile-conversation-fork-agent.spec.ts` / mobile-chrome | 001.1-001.3, 004.3, 004.6-004.7, 006.1 |
| `preview range and selected evidence match delivered context` | same | same | 001.5, 002.1-002.6, 003.1-003.2 |
| `model changes preserve prompt and allow oversized context submission` | same | same | 003.3-003.6, 006.4 |
| `expired draft can rebuild without losing instruction` | same | same | 001.4, 005.2, 006.4 |
| `touch preview has one scroll owner and returns focus` | keyboard counterpart in same desktop file | same phone file | 006.1-006.5 |
| `new task keeps fork after create without start and reload` | `conversation-fork-tasks.spec.ts` / chromium | `mobile-conversation-fork-tasks.spec.ts` / mobile-chrome | 004.1-004.6 |
| `child task keeps parent and explicit workspace choice` | same | same | 004.1-004.2, 004.7-004.8, 006.1-006.5 |
| `creation retry does not create a duplicate destination` | same | same | 004.5, 005.4 |

Phone tests use tap interactions, full preview scrolling, actual 44-pixel targets, safe-area clearance, and zero document horizontal overflow.
Check just below and above the phone breakpoint for composition changes.
Test a non-primary source session so existing parent-context heuristics cannot choose the wrong transcript.
Test same-task workspace sharing through the existing environment identity, without resetting files.
For a new task, assert a different physical workspace and no source uncommitted files.
Assert the ordinary repository/base defaults remain unchanged, rather than adopting a fork-specific source commit.
For a child task, exercise both modes. Reject an executor that cannot provide the required isolation.

## Work orders

Execute sequentially. No subagent delegation is authorized.

- [x] [Task 01: Compile and preview immutable snapshots](task-01-snapshot-preview.md)
- [x] [Task 02: Preserve selected attachments](task-02-attachment-copies.md)
- [x] [Task 03: Bind snapshots to destination launches](task-03-destination-admission.md)
- [x] [Task 04: Deliver the preview and new-agent flow](task-04-agent-fork-ui.md)
- [x] [Task 05: Deliver task and child-task forks](task-05-task-fork-ui.md)

Dependency order: 01 -> 02 -> 03 -> 04 -> 05.

## Verification results

The design package passed catalog, traceability, link, and specification checks before implementation. The implementation checks passed as follows:

- Backend conversation-fork service, handler, SQLite, and orchestrator tests passed. PostgreSQL snapshot and attachment tests passed against the disposable PostgreSQL 16 instance.
- The saved-prompt, initial-task-brief, and deferred-launch regressions passed. `go test ./internal/backendapp` and `go build ./...` passed.
- The focused frontend suite passed 141 tests across 14 files. `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Managed desktop E2E passed 3 tests across agent, new-task, and child-task flows. Managed mobile E2E passed 3 tests across the same destinations, including create-without-start and later launch.
- Targeted ESLint passed with zero errors and warnings after extracting the dialog surface into its own component.
- Public documentation validation passed: 62 validator tests and 47 published pages. Catalog validation and full specification lint passed.
- `git diff --check` passed.

The Traditional Chinese conversion command passed after adding a reviewed override for the existing `workflows:openAgentSettings` phrase. Fork translations were generated with the repository converter while preserving other reviewed catalog text.

All five work orders are done. Requirements are active and the system design is current.

### Review remediation verification

The six direct review findings and subsequent PR findings are resolved in the working tree. A task's permanent fork provenance no longer flows into later session delivery metadata; only its first atomic pending-snapshot claim enables delivery. Selected copied attachments reach agent, immediate-task, and delayed-task launch inputs and share the normal aggregate limits with new uploads. Separate-workspace creation rejects Local executors. Replacement previews preserve the previous usable snapshot on failure and clean up stale drafts. Accepted user cutoffs work during an active assistant turn. Known model limits appear from models.dev metadata, while estimates remain informational. Fork retries preserve the exact compiled bytes, isolate historical text from trusted system context, and reuse the first user message without duplicating the fork block. Destination receipts distinguish completed setup from retryable partial creation; rollback restores staged copies and the draft. Expiry and discard release private copied files and compiled payloads. Public guidance, requirements, system design, and test traceability match these behaviors.

Verification passed after remediation:

- Full Go tests passed for `internal/orchestrator`, `internal/task/service`, `internal/task/handlers`, and `internal/task/repository/sqlite`, with PostgreSQL tests enabled through `KANDEV_TEST_POSTGRES_DSN`.
- `go build ./...` passed.
- `pnpm exec vitest run hooks/domains/task/use-conversation-fork.test.ts` passed all 9 tests. `pnpm run typecheck` and `pnpm --filter @kandev/web build:vite` passed.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, catalog validation, full spec lint, public-doc validation (62 tests and 47 pages), and `git diff --check` passed.
- Targeted ESLint passed with zero errors and warnings after extracting the dialog surface into its own component.

## Risks

- A multipage read can mix source revisions. The compiler needs its dedicated transaction-bound read.
- Snapshot copies add storage only when users create forks. Quotas and expiry must include attachment cleanup.
- The initial-message boundary spans several launch paths. A generic string prepend can duplicate or discard context.
- Source attachments have strict claim scopes. Reusing their IDs would fail or cross an ownership boundary.
- Model metadata can be unavailable before launch. Estimates must remain explicit about unknown overhead.
- Hidden saved-prompt expansions are absent from portable history. This omission must remain visible.
- Existing companion plans retain their historical pending statuses; this feature was validated directly against their current contracts.
- A task README links a missing bounded-session-history design. This package relies on the present source and reconciliation decision instead.

## Documentation impact

Task 05 added a how-to section to `docs/public/tasks-and-workflows.md` after desktop and phone flows passed.
It checked `docs/public/agents-and-profiles.md`, `README.md`, and `docs/screenshots.md` for conflicting fork or context claims; no changes were needed there.

## PR fixup status

PR #3897 is updated with commit `0d567522d20f2d3b4ba316fa717f65dcde284414`. The review fixes cover one-shot fork delivery, copied attachment admission and launch, executor isolation, retryable draft replacement, active-turn user cutoffs, known model limits, destination receipts and rollback, nested history, prompt trust boundaries, and expiry/discard cleanup.

The exact-head GitHub run finished with 60 checks passed, 0 failed, and 0 pending. All actionable review threads are resolved. The desktop and phone form/preview screenshots were recaptured after the final code commit and published at media commit `b600d5b3ec32f73eadd837bd5bbf85124d94fff4`; the PR body was verified unchanged outside its screenshot sentinels. The synthetic merge check against current main (`9885fd40baa4396c5098e3e81cd257bbe733ab0a`) returned tree `efba63e753b3b068e4b4f943938c819ffcfc23ad` without conflicts. The PR remains open and unmerged.

---
created: 2026-10-05
status: complete
requirements:
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-001
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-002
  - REQ-PROJECTS-CONTEXT-KNOWLEDGE-003
  - REQ-PROJECTS-AGENT-PROJECTS-001
  - REQ-PROJECTS-AGENT-PROJECTS-004
  - REQ-PROJECTS-AGENT-PROJECTS-005
system_design:
  - ../../specs/projects/system-design/context-knowledge.md
  - ../../specs/projects/system-design/agent-projects.md
legacy_specs: []
---

# Implementation Plan: Agent Projects Refinements

## Overview

Give Projects OKF starter files, index-aware instructions, advisory editor
feedback, and the approved creation/navigation UX. Implement provisioning,
guidance, and editor feedback first, then creation and sidebar behavior. Preserve
the raw-text contract and existing project policy throughout.

The source baseline is the current Projects branch at
`514f64bb7b56d79b1aedbc926c0f75a800f2aefc`, associated with PR #3920.
Concurrent commits reached the shared branch during design. Before execution,
read the current head and reconcile any changed context or editor contracts.
The [original Projects work package](../agent-projects/plan.md) and
[QA remediation report](../agent-projects/qa-2026-10-04.md) remain completed
delivery records. This package contains the additional format and UX work.

## Scope

### In scope

- Missing-only starter files with an OKF versioned index and structured notes.
- Existing plain Markdown, empty files, custom indexes, and unfamiliar metadata.
- Common index-first reading guidance and role-specific authoring guidance.
- Local, advisory inspection of the current editor draft.
- Localized feedback, desktop/phone rendered coverage, and public guidance.
- A wider New Task-style creation form and shared remote repository selection.
- Optional initial prompt with idempotent direct-user message submission.
- Collapsed advanced worker profiles, inherited defaults, and accessible help.
- Default Projects ordering below Integrations, empty collapse, and hidden archive entry.

### Out of scope

- Format-dependent API rejection or a database metadata projection.
- Automated conversion, knowledge extraction, imports/exports, or search.
- A preview renderer, clickable Markdown links, or new editor controls.
- Automatic provenance, verification, attested computation, or trust scores.
- Storage topology, concurrent-write policy, permission, or feature-flag changes.

## Inputs and settled decisions

- [Requirements](../../specs/projects/requirements/context-knowledge.md)
- [System design](../../specs/projects/system-design/context-knowledge.md)
- [Project setup/navigation requirements](../../specs/projects/requirements/agent-projects.md)
- [Project setup/navigation design](../../specs/projects/system-design/agent-projects.md)
- [Format decision](../../decisions/2026-10-05-project-context-knowledge-format.md)
- [Storage decision](../../decisions/2026-09-24-agent-project-identity-and-context.md)

The user selected incremental OKF adoption. Source inspection confirms the
existing filesystem store and raw editor can preserve this format. Routine
design choices are missing-only provisioning, a browser helper using the
installed YAML library, and compact inline advice. No material question remains.
The user subsequently approved the ASCII creation/sidebar preview and explicitly
requested full implementation by one GPT-6 Luna xhigh worker, coordinator review,
fixes, and push to PR #3920. The primary session owns integration and delivery.

## Technical approach

### Provision useful files without rewriting context

Change `ContextStore.Provision` in `context_store.go`. Replace the notes-only
early return with independent missing-file creation for `notes.md` and
`index.md`. Use existing no-follow handles and exclusive `CreateFile`. Tests
must cover preservation and races rather than comparing a production constant
with itself. There is no startup migration or new launch provisioning call.

### Refresh project guidance through the existing seam

Extend `AgentProjectInstructions` in `sysprompt.go`. Read the index before relevant
knowledge, explain the missing-index fallback, and describe concept metadata.
Keep coordinator status concise and maintain index entries. Preserve existing
role/tool constraints and the lifecycle instruction refresh.

| Surface | Identity and transport | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| Coordinator | Stored coordinator tier, existing lifecycle launch/resume | Common reading guidance plus notes/index maintenance | Prompt tests and lifecycle refresh test. Missing index falls back to notes. |
| Economy and frontier worker | Stored worker tier, same lifecycle seam | Common reading guidance plus assigned-task boundaries | Table-driven prompt tests. No coordinator tools or role granted. |
| Ordinary task or Office | Existing non-project identity | No project format guidance | Preserve existing guard and system-content tests. |
| Unsupported project profile/executor | Existing admission policy | Reject before launch under the existing access contract | Existing project eligibility regressions remain in the package tests. |

Shared instruction code does not expand supported agent profiles or executors.
Tests prove instruction wiring and available files, not a model's adherence.

### Inspect the current draft inside the editor

Add `lib/agent-projects/context-format.ts` and focused tests. Use `yaml` document
nodes for bounded, read-only inspection. Emit domain discriminants and diagnostic
codes, then translate them in the context panel. Do not stringify YAML or alter
the draft. Recognize reserved files before applying the concept `type` rule.

Memoize from path and draft. Feedback stays within the existing file editor.
The API types, endpoints, hook save contract, and file request count stay intact.
Retain current conflict and request-generation behavior.

### Record the public contract after implementation

Update the Agent Projects section of `docs/public/tasks-and-workflows.md` with
the entry point, a small concept example, legacy compatibility, and advice limits.
The page remains an explanation with its existing workflow guidance.
Update `apps/backend/internal/projects/AGENTS.md` to name the format contract and
additional work-package path. After all checks pass, promote the new requirement
and design statuses and record the actual work-order results.

### Create and start through existing task/message APIs

Reuse New Task's remote-picker primitives and provider identity/inspection APIs.
Add the metadata-only remote-selection inspect/register seam defined in the
Project system design. Reuse provider clients and the repository identity lock;
keep generic repository creation's clone-URL boundary unchanged. Server
verification owns canonical identity, exact clone URL, and default branch for
built-in and plugin providers. Resolve repository records once per submission, upsert them into workspace
state, and apply the shared Projects remote eligibility rule. Resolve worker
profile inheritance to concrete valid IDs. Keep project creation's request key,
then ensure its coordinator session and use the existing `sendMessageRequest`
with a stable caller message ID for nonblank prompts. Cache the saved project
and message payload for retries. Do not introduce a project launcher or a
database prompt field. Preserve draft and saved coordinator on partial failure.

### Respect sidebar preferences and Office ownership

Reorder only the default ordinary navigation branch. Use data-dependent Projects
expansion when the stored section map has no choice. Remove the unconditional
global default that would mask that fallback; retain Office's explicit default.
Keep the create action visible while closed. Hide archived controls and their
eager sidebar load while retaining archive/restore APIs and lifecycle tests.

## ASCII UI preview

The changed region is the existing context editor. Labels are illustrative and
must be localized. Structural requirements are toolbar, wrapping format advice,
and the raw editor in that order. No new interaction or overlay is required.

### UI-01: Desktop context file with advisory feedback

Entry: task Files -> Context -> `notes.md`.
Maps to `AC-PROJECTS-CONTEXT-KNOWLEDGE-003.1` through
`AC-PROJECTS-CONTEXT-KNOWLEDGE-003.6`.

```text
Files / Context
[Back]  notes.md                              [Save]
Structured Markdown
Type is missing. You can still save.
+-------------------------------------------------+
| ---                                             |
| title: Project notes                            |
| ---                                             |
| # Project notes                                 |
| ...                                             |
+-------------------------------------------------+
```

The toolbar remains fixed within the panel. Actions retain 28px desktop height.
Advice wraps without its own scrollbar. The textarea owns content scrolling.
The Save action depends on dirty/busy state, not format advice.

### UI-02: Phone context file with the same advice

Entry: focused mobile Files -> Context -> `notes.md`.
Maps to `AC-PROJECTS-CONTEXT-KNOWLEDGE-003.5` and
`AC-PROJECTS-CONTEXT-KNOWLEDGE-003.6`.

```text
[Back]  notes.md             [Save]
Structured Markdown
Type is missing.
You can still save.
+--------------------------------+
| ---                            |
| title: Project notes           |
| ---                            |
| # Project notes                |
| ...                            |
+--------------------------------+
```

Reuse `AgentProjectContextFile` in the current focused mobile Files surface.
Actions keep at least 44px touch targets. Advice wraps and the textarea remains
the content scroll owner. Preserve surrounding safe-area and viewport behavior.
Editing advice must not remove keyboard focus. Long localized text must remain
contained at phone width.

For structured notes, the format row reports detected OKF metadata without
certifying knowledge. For legacy content, it reports plain Markdown without
warning. A storage conflict uses the existing error region and retains the draft.

### UI-03: Desktop project creation

Entry: ordinary sidebar Projects -> plus. Maps to
`AC-PROJECTS-AGENT-PROJECTS-001.6` through `001.9` and `005.2`.

```text
+--------------------------------------------------------------------------+
| Create project (i)                                                   [X] |
|                                                                          |
| Repositories                                                             |
| [acme/api v] [x]  [acme/web v] [x]  [+ Add remote repository]             |
|                                                                          |
| Project name                                                             |
| [Account settings                                                      ] |
|                                                                          |
| Initial prompt (optional)                                                |
| +----------------------------------------------------------------------+ |
| | Review the account settings flow and coordinate the implementation.  | |
| |                                                                      | |
| +----------------------------------------------------------------------+ |
|                                                                          |
| Coordinator profile                       Primary repository             |
| [Selected profile v]                      [acme/api v]                    |
|                                                                          |
| > Advanced settings                                                      |
|                                              [Cancel] [Create and start] |
+--------------------------------------------------------------------------+
```

Use the shared 900px desktop form, 85vh height bound, spacing tokens, and a
single body scroll. Blank prompt changes the action to Create project.
Advanced starts closed; expanding shows economy/frontier selectors side by side,
each with an info trigger and Same as coordinator default, then execution info.
The shared repository picker shows provider tabs, search/paste, selected marks,
and errors/retry. No setup detour through repository settings is required.

### UI-04: Focused phone creation and help

```text
[Back] Create project                  (i)
Repositories
[acme/api v] [x]
[+ Add remote repository]
Project name
[Account settings                       ]
Initial prompt (optional)
+---------------------------------------+
| Review the account settings flow...   |
|                                       |
+---------------------------------------+
Coordinator profile
[Selected profile v                     ]
Primary repository
[acme/api v                             ]
> Advanced settings
-----------------------------------------
[             Create and start           ]
                 safe area
```

The content body scrolls and the footer stays visible. Fields stack, chips wrap,
and actions have 44px targets. Tap an info trigger to read a dismissible bottom
help drawer. Desktop help works on hover and keyboard focus. Dismissal restores
focus to the opener. Review a narrow phone and coarse-pointer tablet.

### UI-05: Default sidebar

Maps to `AC-PROJECTS-AGENT-PROJECTS-005.5` and `005.6`.

```text
AUTOMATIONS                            >
INTEGRATIONS                           >
PROJECTS                           [+] >
TASKS                                  v
  Ordinary task
```

An empty section has only the header. Its plus works while closed. With projects
and no saved choice, the section can expand to its existing project/worker rows.
There is no archived-projects row. Preserve explicit expansion and custom layout
choices; Office's project navigation is unchanged.

## Tests

Proposed test names identify the required evidence. Implementation can group
table cases while preserving this coverage.

| Acceptance | Test location and evidence |
| --- | --- |
| `001.1` | `context_store_test.go`: `TestContextStoreProvisionOKFStarterFiles`. Assert index version/link and notes metadata/sections. |
| `001.2`, `001.3` | `context_store_test.go`: `TestContextStoreProvisionPreservesExistingFiles`, `TestContextStoreProvisionConcurrentCreators`. Cover empty, custom, partial, and plain files. |
| `001.4`, `001.5` | Inspector custom-type/extension cases, existing raw-write regressions, and browser save/read equality. |
| `002.1`, `002.3`, `002.4`, `002.5` | `sysprompt_test.go`: `TestAgentProjectInstructionsKnowledgeGuidance` across all three roles. |
| `002.1`, `002.2` | `manager_launch_test.go`: extend `TestApplyAgentProjectInstructionsAddsCurrentWorkspaceAndRefreshesMetadata`; assert fresh instruction snapshot without bundle bodies. |
| `003.1`, `003.2`, `003.3`, `003.4` | `context-format.test.ts`: reserved files, custom types, missing/invalid type, YAML errors, versions, BOM/CRLF, and header limit. |
| `003.5`, `003.6` | Projects desktop/mobile E2E plus existing hook conflict and stale-response tests. |

Each shortened acceptance suffix above refers to
`AC-PROJECTS-CONTEXT-KNOWLEDGE-<suffix>`.

## E2E tests

Extend the existing Projects specs. Use the real isolated backend and mocked
external agents/services, with the existing request/event waits.

- `tests/projects/agent-projects.spec.ts`, project `chromium`: read starter index
  and structured notes, then edit custom metadata and inspect draft advice.
  Save malformed or plain text and observe the same content from a worker.
  Exercise an intervening API write and preserve the failed-save draft.
  Context files remain absent from repository Changes.
- `tests/projects/mobile-agent-projects.spec.ts`, project `mobile-chrome`: open
  the same starter files through focused Files and read wrapping advice.
  Repair and save a header, then save legacy Markdown and observe worker visibility.
  Measure Back/Save hit targets and validate editor focus and viewport containment.
- Keep lifecycle and navigation flows, including project archive/restore and
  explicit context deletion. Restore through the API when the archive row is hidden.
- Create with prompt and without prompt; inspect one direct coordinator prompt
  and no unsolicited blank-prompt start. Retry lost responses and start failures.
- Pick a provider repository and paste a URL, remove/change primary, inspect
  collapsed advanced defaults, overridden profiles, keyboard/tap help, and errors.
- Measure dialog/footer/control geometry and capture real desktop/phone screenshots.
- Verify default section order, empty collapse/create, persisted choices, custom
  layout preservation, no archived row, and Office navigation regression coverage.

Browser evidence covers `AC-PROJECTS-CONTEXT-KNOWLEDGE-001.1`, `001.4`, `001.5`,
and all six `003` criteria. Legacy upgrade and instruction-refresh behavior have
direct unit/integration evidence instead of an artificial browser restart flow.

## Work orders

- [x] [Task 01: Safe OKF starter files](task-01-starter-files.md)
- [x] [Task 02: Index-aware project instructions](task-02-agent-guidance.md)
- [x] [Task 03: Advisory editor feedback and browser evidence](task-03-editor-feedback.md)
- [x] [Task 04: Project creation UX and initial prompt](task-04-project-creation.md)
- [x] [Task 05: Compact project navigation and delivery evidence](task-05-project-navigation.md)

Execute sequentially in the explicitly authorized GPT-6 Luna xhigh worker.
The coordinator reviews the resulting code and actual rendered UI, requests fixes
from the same worker, verifies results, and commits/pushes the final changes.

## Verification results

Implementation: complete after the user's explicit request. All five work orders
were executed sequentially by the authorized GPT-6 Luna xhigh worker. The primary
session reviewed production code and actual desktop, phone, and tablet rendering,
coordinated the repairs, and owns commit/push and CI delivery.

Final local verification on 2026-10-05:

- Eight affected backend packages pass, plus the remote-registration convergence
  race test. Backend lint reports zero issues and the full backend build passes.
- Shared Project and ordinary-task compatibility tests pass: 98 tests. Sidebar
  and UI-state regressions pass 530 tests, with 113 focused preference tests.
  The deferred concurrent-refresh regression is Red then Green (three tests).
- Final managed desktop Projects passes seven of seven cases; phone Projects
  passes two of two. These include real backend/SQLite/Git integration with
  mocked external providers/agents, prompt and blank-prompt creation, URL lookup,
  profile inheritance/pinning, context sharing/conflicts, archive/restore,
  deletion, live navigation, rotation, and phone/fine-pointer/tablet geometry.
- The production Vite build passes. All 40 changed TS/TSX paths pass lint;
  48 source/locale files pass formatting. Typecheck, complete translations,
  translation ratchet, and E2E wait guard pass.
- CI exposed a closed-form repository snapshot loop (QA-36). Three real-store
  regressions are Red then Green after the stable empty-array repair. The
  corrective production build, focused lint/typecheck/formatting, desktop ten
  cases, and phone three cases pass. These include ordinary Settings with
  Projects disabled and the full seven/two Projects flows, without retries.
- Public documentation passes 62 tests and validation of 47 pages. Catalog/spec,
  architecture, changed harness, local links, and whitespace checks pass.
  Offline PR documentation coverage covers all five refinement work orders.

Findings, corrections, interim failures, manual geometry, and retained receipts
are recorded in [the final QA report](qa-2026-10-05.md). Exact pushed head and
current-head CI are recorded in the Kandev task plan and PR #3920 after delivery.

Design validation on 2026-10-05:

- `python3 scripts/list-docs.py validate`: passed, with 353 decisions and 1353
  specification files.
- `python3 scripts/lint-spec-files.test.py`: passed all 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Offline `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: `covered`,
  with all three work orders and zero errors. Inputs included the planned
  runtime paths and current workspace documents, including untracked files.
  This validates the package without publishing a GitHub status.
- Local Markdown links and whitespace: passed for all seven new documents.
- `git diff --check -- docs/specs docs/decisions docs/plans/agent-project-context-knowledge`:
  passed.

The earlier design handoff remains a historical result. The approved package now
includes five work orders; refresh coverage validation for the added UX scope.

## Risks

- An early return can leave `index.md` missing when notes already exist.
- An automatic rewrite can discard legacy notes or unknown metadata.
- A serializer can change comments, ordering, hashes, or user text.
- A validation message can accidentally disable recovery saves.
- Unbounded YAML or full-body parsing can interrupt typing in large files.
- Prompt wording can accidentally change worker tools or trusted boundaries.
- Advice can reduce usable editor height or overflow on phones.
- Repository resolution or a lost response can create duplicate records or prompts.
- An inherited worker selector can overwrite an existing explicit profile on edit.
- Persisted sidebar defaults can mask empty collapse or change Office navigation.
- A shared picker can accidentally introduce workflow/branch controls into Projects.

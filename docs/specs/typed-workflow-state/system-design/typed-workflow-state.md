---
status: draft
system: typed-workflow-state
requirements:
  - REQ-TWS-001
  - REQ-TWS-002
  - REQ-TWS-003
  - REQ-TWS-004
  - REQ-TWS-005
---

# Typed workflow review state system design

## Purpose and boundaries

This design records the evidence the requirements were written against: prior
positions already taken, what comparable products shipped, the sampled shapes of
every input the build will touch, and the E2E decision. It defines no behaviour of
its own; the acceptance criteria under `../requirements/` are authoritative.

Backend only. No workflow YAML, no plan write API change, no frontend change.

## Prior art

### Leg 1 — our own prior reasoning (wiki)

**Receipt.** Vault resolved through the local Obsidian wiki config to the author's
personal wiki vault, pinned for this query with the named route `@henry` so the
lookup did not depend on whichever vault was last activated; QMD collection `wiki`.
Queried through the `qmd` MCP server (semantic, not the grep fallback) with a
three-part lex/vec/hyde document on review round counts, findings ledgers and
typed-versus-prose agent state, plus a follow-up lex query for `ladder inversion`.

The wiki already holds a position on this design, written 2026-09-01:
the vault-relative page `concepts/artifact-write-api.md`
(`lifecycle: draft`, `base_confidence: 0.85`). It independently reproduces the
12-versus-5 measurement above and takes three positions this spec follows rather
than re-derives. **"Control flow parsing the artifact"** is a named anti-pattern —
*"the routing is already returning wrong answers"* — and Part 1 is its prescribed
remedy, the ledger already carrying a build-failing writer pin so *"the true count
is one `COUNT(*)` away."* **"A text-derived state machine ... returns wrong answers
silently, and no test can catch it because there is no code"** is why Part 1 is
scoped as a correctness fix, and why the zero-row and boundary cases below are
mandatory: once the count is in Go, tests *can* catch it. **"Append without
curation"** settles whether resolution needs its own tool — **it does**, since a
ledger an agent can write and read but never close is not an improvement over
prose, so `resolve_review_finding_kandev` is in scope (REQ-TWS-004), not deferred.
The page frames both parts as **ladder inversion**: *"using a model to judge
something a query could decide."*

**Departure.** The page's headline concern is the plan write API itself
(replace-only, no base fingerprint). This card deliberately does **not** touch it;
see Out of scope 2.

### Leg 2 — what other products shipped (saas-kb)

**Receipt.** `search_fsm_docs`, `category: "ai_sdlc"`, three queries: *"agent review
loop iteration limit round count structured findings"*, *"maximum iterations loop
limit stuck detection agent stop condition"*, *"read back review comments unresolved
threads tool list"*. Scores were low (0.009–0.016), so two documents were read in
full rather than trusted from snippets. Both are vendor claims about what a product
does, not evidence it works.

- **OpenHands `StuckDetector`** (`sdk_guides_agent-stuck-detector.md`) computes the
  loop-bound **host-side from the event history**; the agent never asserts its own
  iteration count. Same position as Part 1: the host owns the counter because it owns
  the log.
- **Devin Review** (`work-with-devin_devin-review.md`) types findings server-side and
  makes **resolution a first-class state transition** — corroborating Leg 1 that a
  findings store needs a close path. It caps repeated passes with a per-PR spend
  limit enforced by the platform, not the agent.

**What we are doing differently.** Both vendors bound the loop by *behaviour*
(repetition detected, spend exhausted). We bound it by *structure* — committed
entries into a specific step, from an append-only ledger whose writer set is pinned
by an AST test. Narrower and less clever, but deterministic and auditable, which
repetition heuristics are not. We also inject the number rather than terminating the
run: the cap stays the prompt's policy; this card only makes the number true.


## Input inventory

Sampled 2026-09-01 against commit `e6f98d996` — the base the
typed-workflow-state implementation was written against — and a read-only handle
on the default local Kandev store (`<home>/data/kandev.db`).

Every `file:line` citation in this section is relative to `e6f98d996`, and every
path below is relative to the Go module root `apps/backend/`. Several of the cited
files are edited by that implementation, so the numbers are pinned to the commit
rather than to whatever `main` holds when the doc is read. Resolve one with
`git show e6f98d996:apps/backend/<path>` — the `apps/backend/` prefix is required,
since a repo-root-relative path does not exist in the tree.

### Part 1 inputs

`sysprompt.InterpolatePlaceholders(template, taskID string) string`
(`internal/sysprompt/sysprompt.go:501`) is four lines and does one
`strings.ReplaceAll` of `{task_id}`. It is pure — no context, no database handle —
so any count must be computed by the caller and passed in. Three existing tests
cover it (`internal/sysprompt/sysprompt_test.go:585-598`).

Exactly **two** production call sites, both in
`internal/orchestrator/task_operations.go`, both already holding `ctx`, `step` and
`taskID`. The **enclosing** function is named below, which is not the same as the
entry point: `buildWorkflowPromptWithContext` (`:1935`) is a three-line wrapper that
only delegates to `buildWorkflowPromptWithTrustedContext` (`:1952`), so it carries
neither interpolation itself.

| Line | Enclosing function | Template |
|---|---|---|
| 1970 | `buildWorkflowPromptWithTrustedContext` | `step.Prompt` — the step prompt |
| 2015 | `workflowInstructionsBlock` | the workflow-level prompt |

They are **not independent**: `workflowInstructionsBlock` is called from
`buildWorkflowPromptWithTrustedContext:1964`, six lines above the other site, so one
prompt build performs both interpolations (NFR-1).

`buildWorkflowPromptWithContext` is reached from three places, and in **all three**
the task is already at `step` with the current entry's ledger row committed before
the prompt is built: `event_handlers_workflow.go:2657/2673/2718/…` via
`processOnEnter`; `task_operations.go:2374`, right after
`s.advanceTaskWorkflowStep(...)` at `:2372`; and `task_operations.go:1855`, inside
`applyWorkflowAndPlanModeWithPromptContext` (reached from `applyWorkflowAndPlanMode`
at `:1805`), on session launch.

What makes that true is the write, not the call path: `recordStepTransition` runs on
the **same transaction** as the `UPDATE tasks … workflow_step_id`
(`internal/task/repository/sqlite/task.go:639`, immediately after the update at
`:630`), so any path that has moved the task to `step` has already committed the
row. Do not look for the guarantee in a step-entry identifier passed down the call
chain. Two unrelated values in this area are both called `entryID`: a string
`formatEntryID(transitionID)` derived from the ledger row, and the `int64`
`workflow_step_entries` row id allocated by `allocateStepEntryIfPending`
(`step_entries.go:121`). `processOnEnter` receives the **second**, which belongs to
the on-enter dispatch marker system, is a different table from
`task_step_transitions`, and is `0` on the legacy path
(`event_handlers_workflow.go:2051`). It evidences nothing about the ledger.

`task_step_transitions`
(`internal/task/repository/sqlite/base_schema.go:824-858`) is append-only.
`recordStepTransition` (`internal/task/repository/sqlite/step_transitions.go`) is
the sole writer; it is a **no-op when `fromWorkflowStepID == toWorkflowStepID`**
(position-only reorder, re-issued move to the current step — *not* a later return
to the step, which does write a row); the seven functions mutating
`tasks.workflow_step_id` are pinned by `TestStepTransitionWritersArePinned`, an
AST walk over the backend tree. Indexes:
`(task_id, occurred_at, id)` and `(occurred_at)`; none on `to_workflow_step_id`.
**No production read path exists today** — only
`internal/telemetrycontract/contract.go`, for a health probe.

Live counts: 2,194 rows; earliest `occurred_at` 2026-08-16 14:59:04Z; 908 tasks,
405 with at least one row, **503 (55%) with none** — the zero-row case is the
majority, not an edge case.

Live placeholder usage: **zero** steps and **zero** workflows use `{task_id}`. 114
step prompts contain a brace token and every one is `{{task_prompt}}` — which
matters, because the prompt build tests for `{{task_prompt}}` **after**
interpolation, one line below the step-prompt call site
(`buildWorkflowPromptWithTrustedContext`, `task_operations.go:1971`), so damaging it
would silently drop the base prompt.

### Part 2 inputs

`publish_review_findings_kandev` is registered in the `review` group, kanban
surface only (`internal/mcp/server/server.go:1007`, `:1812`). `registerReviewTools`
registers **exactly one** tool; no `list_` or `get_` counterpart exists anywhere.
`registerReviewHandlers` (`internal/mcp/handlers/review.go:39`) registers five WS
actions and returns a hardcoded count.

`task_review_findings` rows are `models.TaskReviewFinding`
(`internal/task/models/models.go:2315-2336`);
AC-TWS-003.4 names the subset the tools return, and `AnchorText`/`FileDiffHash`/
`RepositoryID`/`TaskID`/`UpdatedAt` are deliberately not among them.
`ReviewFindingStatus` has **three** values: `open`, `resolved`, `dismissed`
(`internal/task/models/models.go:2258-2260`).
`Repository.ListTaskReviewFindings`
(`internal/task/repository/sqlite/task_review.go:266`) is task-scoped and orders
by `repository_name, file_path, start_line, id`.

`review.NormalizeFindingInput` (`internal/review/parse.go:162`) trims and lower-cases
the publisher's `severity` before `models.ValidReviewSeverity` rejects it, and
defaults an omitted `line_end` to `line`.

**Authorisation is the load-bearing detail.** `ReviewService.authorizeTask` is an
opt-in per-user check wired by `SetTaskAuthorizer`, and `s.authorize` is called in
exactly **one** place: `PublishFindings`, at
`internal/task/service/review_service.go:320`.
`GetTaskReview` (`:505`) and `UpdateFindingStatus` (`:472`) do **not** authorise —
they are reached today only from browser WS actions, so reusing either from MCP
unchanged would bypass the publisher's reach rule. `UpdateFindingStatus` takes a
`findingID`, not a `taskID`, so a resolve path must resolve the finding to its owning
task before it can authorise. It also stamps `resolved_at` unconditionally
(AC-TWS-004.10).

Existing WS actions: `task.review.get`, `task.review.finding.update` (UI) and
`mcp.publish_review_findings` (MCP). MCP tools use `mcp.`-prefixed actions by
convention, and those are refused on the raw browser WebSocket
(`internal/gateway/websocket/client.go:175`).

Live data: **6 findings total, all `open`**, across 2 tasks — consistent with a store
nothing reads.


## E2E decision input

**User-visible surfaces touched: none. Recommendation: no new E2E specs.** Part 1
changes prompt text only and no live template contains the new token. Part 2 adds
MCP tools; the Review panel reads findings over `task.review.get`, unmodified here,
and no frontend file is touched (Out of scope 9).

Two nuances, stated not glossed. AC-TWS-004.9 makes resolve a new producer
of an existing event (`TaskReviewFindingUpdated`) — the same event on the same
channel the panel's own control already emits, so no new rendering path. AC-TWS-004.10
changes that panel's repeated-resolve behaviour (the timestamp stops moving): a
service-layer change, observable in the panel, with no frontend edit.

No existing E2E spec should change; if one does, that is evidence of unintended
scope.

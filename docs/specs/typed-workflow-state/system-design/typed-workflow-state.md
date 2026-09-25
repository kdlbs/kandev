---
status: draft
system: typed-workflow-state
requirements:
  - REQ-TWS-001
  - REQ-TWS-002
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
*"the routing is already returning wrong answers"* — and REQ-TWS-001 is its
prescribed remedy, the ledger already carrying a build-failing writer pin so *"the
true count is one `COUNT(*)` away."* **"A text-derived state machine ... returns wrong answers
silently, and no test can catch it because there is no code"** is why this system
is scoped as a correctness fix, and why the zero-row and boundary cases below are
mandatory: once the count is in Go, tests *can* catch it. **"Append without
curation"** bears on whether a findings ledger needs a close path — it does, since
a ledger an agent can write and read but never close is no improvement over prose.
That position stands, but this system no longer acts on it: the read and resolve
tools it justified were withdrawn on upstream review and the whole capability moved
to a successor contract (README Out of scope 7). The page frames the round count as
**ladder inversion** — *"using a model to judge something a query could decide"* —
which is precisely REQ-TWS-001's argument and is unaffected by that withdrawal.

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
  iteration count. Same position as REQ-TWS-001: the host owns the counter because
  it owns the log.
- **Devin Review** (`work-with-devin_devin-review.md`) caps repeated passes with a
  per-PR spend limit **enforced by the platform, not the agent** — the same division
  of labour REQ-TWS-001 assumes, and the one the upstream review asked for when it
  said the workflow engine should own the retry limit (README Out of scope 4). It
  also types findings server-side and makes resolution a first-class state
  transition, corroborating Leg 1's close-path position; that half informs the
  successor contract rather than this system.

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

### Step-entry number inputs

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

## E2E decision input

**User-visible surfaces touched: none. Recommendation: no new E2E specs.** This
system changes prompt text only, and no live template contains the new token, so
there is no rendered surface an assertion could reach. No frontend file is touched
(README Out of scope 6) and no existing event gains a new producer.

No existing E2E spec should change; if one does, that is evidence of unintended
scope.

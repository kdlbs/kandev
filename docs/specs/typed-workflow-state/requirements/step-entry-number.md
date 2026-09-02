---
status: draft
system: typed-workflow-state
created: 2026-09-01
owners:
  - kandev
---

# Step-entry number requirements

Part 1 of the typed workflow review state system: a server-computed step-entry
number substituted into workflow prompt templates, so the agent never counts its
own prose. System-wide terminology, non-functional constraints and exclusions are
in [../README.md](../README.md).

## Why

A board workflow keeps three things in the task plan as prose that have typed
homes: the review round count, the findings ledger, and the verdicts.

The round cap is enforced today by instructing the agent to count plan lines
beginning with `## SPEC REVIEW VERDICT:` — a string that appears in no Go, TS or
Markdown file here. The agent counts its own prose. Measured against the live
database (2026-09-01) that count is already wrong: task `e6177a00` carries **12
verdict headings** against **5 real transitions into its Spec Review step**, so a
5-round cap would have fired more than twice too early, and three further tasks carry 6 to 8 headings against **zero** ledger rows. Part 1 is therefore a **correctness fix, not a token optimisation** — expect
no measurable token change from it.

Part 2 closes a round trip: a review step can publish structured findings today,
and the step it bounces back to has no MCP path to read them. That gap is why the
findings ledger lives in plan prose.


## Requirements

### REQ-TWS-001: Server-computed step-entry number in workflow prompt templates

Workflow prompt templates can request the number of the current step entry, and
the server substitutes it. The agent never counts.

- **AC-TWS-001.1:** When a workflow prompt template contains the token
  `{step_entry_number}`, the built prompt shall contain the decimal entry number
  in its place, with no braces.
- **AC-TWS-001.2:** The entry number shall be **1-based and inclusive of the
  current entry**: the first time a task is at a step the value is `1`, and on the
  fifth entry it is `5`. A prompt reading "this is entry N" is correct as written.
- **AC-TWS-001.3:** The entry number shall be derived from
  `COUNT(*) FROM task_step_transitions WHERE task_id = ? AND to_workflow_step_id = ?`
  for the task and the step the prompt is being built for.
- **AC-TWS-001.4:** When the recorded entry count is `0`, the substituted value
  shall be `1`. A task whose prompt is being built is at that step, so it has
  entered at least once; `0` would be false and would render "entry 0".
- **AC-TWS-001.5:** Substitution shall apply at both production call sites — the
  step prompt and the workflow-level prompt — so a template works wherever it is
  authored.
- **AC-TWS-001.6:** When the template does not contain `{step_entry_number}`, no
  count query shall be issued. Prompt building happens on every step entry and every
  session launch, so a template that does not ask must not pay (NFR-1).
- **AC-TWS-001.7:** The substituted value shall be rendered in base 10 with no
  thousands separator, no sign, and no leading zeros: `1`, `5`, `12`.
- **AC-TWS-001.8:** When the task identifier or the step identifier is empty, no
  count query shall be issued and the substituted value shall be `1`. The ledger
  normalises `""` to `NULL`, so the query could only return `0`, which AC-TWS-001.4
  already floors at `1`.
- **AC-TWS-001.9:** `{task_id}` and `{step_entry_number}` may both appear in one
  template and shall not interact. Neither substituted value can contain a brace
  token, so the order in which the two are applied is not observable.
- **AC-TWS-001.10:** Substitution shall replace **every** occurrence of the exact
  literal `{step_entry_number}`, matching `{task_id}`'s existing multi-occurrence
  behaviour. No other form is recognised, so `{{step_entry_number}}` renders as
  `{5}` on entry 5 — the inner literal matches, the outer braces remain. Accepted
  rather than special-cased: the token is new, no template uses it, and a doubled
  form has no second meaning to assign. `{{task_prompt}}` is unaffected, containing
  no such literal (AC-TWS-002.5).

### REQ-TWS-002: The entry number degrades visibly, never silently

Every failure mode leaves evidence rather than a plausible wrong number, because
a fabricated value silently disables a cap. AC-TWS-002.1 to AC-TWS-002.5 are
observable at runtime. **AC-TWS-002.6 alone is verifiable by inspection** — it
constrains a doc comment rather than behaviour — and Verification case 16 records
that, so its absence from a runtime assertion is a stated carve-out, not a gap.

- **AC-TWS-002.1:** When the count query returns an error, the token
  `{step_entry_number}` shall be left in the prompt **verbatim**, and the failure
  shall be logged at warn level with the task and step identifiers. An
  un-substituted token is visible to a reader; an invented number is not.
- **AC-TWS-002.2:** A count query failure shall not fail prompt building. The
  rest of the prompt shall be built and delivered unchanged.
- **AC-TWS-002.3:** Tokens other than `{task_id}` and `{step_entry_number}` shall
  pass through byte-for-byte unchanged, so an un-migrated prompt degrades visibly
  rather than rendering an empty cap.

- **AC-TWS-002.4:** `{task_id}` substitution shall be byte-for-byte identical to
  its behaviour before this change, including when the template contains several
  occurrences and when it contains none.
- **AC-TWS-002.5:** `{{task_prompt}}` shall survive interpolation byte-for-byte.
  It is consumed by `buildWorkflowPromptWithContext` **after** interpolation, so
  damaging it silently discards the task's base prompt.
- **AC-TWS-002.6:** The entry number shall be documented — in the doc comment of
  the function that computes it — as a **lower bound**: entries before the ledger's
  first row (2026-08-16) are not recorded and cannot be counted. Under-counting
  makes a cap fire late; over-counting makes it fire early, which is the failure
  this card exists to remove.


## Verification

One line per case; the cited AC is authoritative for the expected value.


1. N recorded entries **including the current one** → `{step_entry_number}` is N.
   "Recorded" means rows already committed to the ledger when the count is read,
   which per AC-TWS-005.2 always includes the current entry's own row. A fixture
   holding N-1 earlier entries plus the current one therefore expects N, never N+1;
   stating it as "N prior entries" would invert the boundary this card exists to fix
   (001.2, .3).
2. Zero ledger rows → `1`, not `0`, no error (001.4).
3. A count-query error leaves the token literal, logs at warn, and still returns the
   rest of the prompt (002.1, .2).
4. `{unknown_token}` survives unchanged (002.3).
5. `{task_id}` byte-for-byte unchanged — the three existing `sysprompt_test.go`
   cases pass untouched (002.4).
6. `{{task_prompt}}` survives and is still detected by
   `buildWorkflowPromptWithContext`, base prompt substituted (002.5).
7. A template without the token issues no query — asserted against a counting fake,
   not inferred (001.6).
8. A second build for the same entry returns the same number (005.1).
9. Both call sites substitute (001.5).
10. Empty task or step identifier → `1`, no query (001.8).
11. A template with both tokens substitutes both (001.9).
12. Every occurrence in one template is substituted (001.10).
13. `{{step_entry_number}}` renders as `{5}` on entry 5, asserted so a change cannot
    make it silently different (001.10).
14. A leave-and-return yields the next number, not the previous (005.1).
15. Both templates carrying the token in one build issue one query each and the
    build completes — the bound NFR-1 states (001.5, .6, 005.2).
16. The doc comment on the function that computes the count states the value is a
    **lower bound**. Checked by reading that comment, not by a runtime assertion:
    the ledger's first row (2026-08-16) is a fact about this database's history
    that no fixture reproduces, so there is nothing to assert against. This case
    exists so the requirement is discharged explicitly rather than by silence
    (002.6).


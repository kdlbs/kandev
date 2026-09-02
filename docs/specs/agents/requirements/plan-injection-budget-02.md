---
status: draft
system: agents
created: 2026-09-01
owners:
  - nova28
---

# Plan injection budget Requirements Part 2

## Overview

This part carries the anti-drift requirement and the exclusions common to both parts
(`## Out of scope`, below). The capability's overview, the terminology every criterion in
both parts judges against, the measurement, and the reducer's own criteria (REQ-001) are
in [Part 1](plan-injection-budget-01.md). Criteria are cross-referenced as `AC-001.n` /
`AC-002.n`, and an `AC-001.n` named below is defined in Part 1. The design is likewise
in two parts: [Part 1](../system-design/plan-injection-budget-01.md) for the design
itself and [Part 2](../system-design/plan-injection-budget-02.md) for Appendices A, B
and C, which is where a bare "Appendix" reference below points.

## Requirements

### REQ-AGENTS-PLAN-INJECTION-BUDGET-002: One reducer, both sites, no drift

**Intent:** The defect fixed here is duplicated, divergent handling of one artifact.
Fixing it in two places recreates it.

**User story:** As an engineer changing how plan context is bounded, I want one place
to change it, so a fix cannot land at one injection site and silently miss the other.

#### Acceptance criteria

- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.1:** The handover site
  (`injectHandoverIfNeeded`) and the dynamic continuation site (`addDynamicPlan`)
  SHALL both obtain their plan text from the single exported reducer, and SHALL NOT each
  carry their own budgeting, truncation, or section logic. Containment (AC-001.14) is
  the ONE permitted asymmetry: it is none of those three, is applied by the handover
  site alone before the reducer runs, and SHALL live in a single shared exported
  function rather than inline at that site.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.2:** Each site's injection path SHALL be
  exercised by a test that drives that path with an over-budget plan and asserts the
  omission notice appears in what the site injects, so a site that stopped bounding its
  plan is a build failure rather than a review note. The test SHALL exercise the SITE and
  not the reducer: asserting the reducer's own behaviour shows nothing about whether a
  site calls it. This is a BEHAVIOURAL guarantee and deliberately not a structural one. A
  site that inlined a byte-identical copy of the reducer would still pass, and catching
  that is NOT required here; no static, import-graph, or AST analysis SHALL be built to
  chase it. What REQ-002 guards against is divergence, and divergence becomes observable
  through this same test the moment such a copy drifts.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.3:** Each site SHALL keep composing its own
  plan document as it does today, and the reducer SHALL be applied to that composed
  text, so under budget each site's injected bytes are unchanged by this capability —
  WITH EXACTLY TWO NAMED EXCEPTIONS, both at the HANDOVER site and neither reachable at
  the dynamic one. (1) Content carrying a `<kandev-system>` start or end literal has it
  removed by AC-001.14 containment; one of 178 stored plans. (2) Whitespace-only content
  injects no plan section, where today the frame is injected around that whitespace
  (AC-001.12); none of 178, but listed because this criterion asserts byte-identity
  universally, not only over plans that exist today. Both are deliberate; there is no
  third.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.4:** The dynamic site's budget SHALL be at
  most `continuationFieldLimit`, so the existing `bounded()` call on `PlanSummary`
  cannot re-cut the reducer's output; a later increase above that limit would
  silently restore head-only truncation and cut the notice off the end, so the
  relationship SHALL be asserted by a test.
  WHERE THAT TEST LIVES: the assertion SHALL live in package
  `internal/agent/runtime/dynamic`, the only place `continuationFieldLimit` is visible;
  the constant SHALL NOT be exported and `bounded()` SHALL NOT be modified (rationale in
  the design's Appendix C). Because `bounded()` trims before its length check, the
  reducer's output SHALL carry no leading or trailing whitespace whenever its input
  carries none — which AC-001.6's assembly rule guarantees. The test SHALL assert
  `bounded(reducerOutput) == reducerOutput` on an output that actually reduced.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.5:** The handover site SHALL apply a budget
  of 12,000 bytes and the dynamic site 4,000 bytes, both as named constants. The
  dynamic value preserves that site's existing limit, so this capability SHALL NOT
  loosen any bound existing today; the handover value bounds a site that has none.
  WHAT THE BUDGET MEASURES: each budget SHALL bound the REDUCER'S OUTPUT — the plan
  document alone — not the text a site wraps around it. The handover frame
  (`"\nThe task has an implementation plan:\n\n%s\n"`) SHALL still be applied, to that
  output, and lies OUTSIDE the 12,000 bytes, as does the `<kandev-system>` wrapper the
  template adds; the dynamic `"Plan summary: "` prefix likewise lies outside its 4,000.
  Tests for AC-001.1, AC-001.2 and AC-002.2 SHALL assert against the reducer's output,
  not the wrapped prompt.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.6:** WHEN the reducer reduces a plan, THEN
  the system SHALL log the site, the task, the input size in bytes, the output size in
  bytes, and the number of whole sections omitted, at `Info`. The sites differ and are
  stated separately: the handover site already emits an `Info` record at injection
  ("injecting session handover context") and SHALL extend it with these fields; the
  dynamic site emits none today and SHALL add one carrying them. The omitted count SHALL
  be the one AC-001.3 defines, on every path. No plan text SHALL be logged.
  THE FIELD NAMES ARE PART OF THIS CRITERION. This is the one place the capability puts
  independently written code at BOTH sites, so REQ-002's single-implementation defence
  does not reach it and an unpinned vocabulary would let the two records drift apart
  unnoticed. Both sites SHALL use exactly these keys: `site`, `task_id`,
  `plan_input_bytes`, `plan_output_bytes`, `plan_sections_omitted`. `site` SHALL carry
  the literal `handover` at the handover site and `dynamic_continuation` at the dynamic
  site, and no other value at either.
  WHEN NO REDUCTION OCCURRED — the plan was absent, or the composed document was empty or
  whitespace-only (AC-001.12), or it was within budget and returned unchanged — THEN the
  three `plan_*` fields and `site` SHALL be ABSENT rather than zero. The handover site's
  existing record still fires in those cases, because it reports the handover and not the
  plan; it SHALL carry exactly the fields it carries today, `task_id` among them, and
  none of the four this criterion adds. The dynamic site SHALL emit no record at all.
  Absent rather than zero is what lets a reader tell "there was no plan to reduce" from
  "the plan reduced to nothing"; `task_id` is unaffected because the handover record
  already carries it for its own reasons.
  WHEN the reducer emitted no plan content because none fits (AC-001.13), THAT IS a
  reduction and SHALL be logged, carrying `plan_output_bytes` of zero and
  `plan_sections_omitted` equal to the document's total section count, no section being
  represented.

## Out of scope

- **Selecting sections by relevance to the incoming workflow step.** Excluded on
  evidence: heading text is uncontrolled agent prose and step names are
  installation-defined (both counted in the design's Appendix A), so a matcher over them
  would make injected content depend on a previous agent's phrasing. A follow-up wants a
  controlled vocabulary — a typed section marker written at plan-write time, or sections
  as addressable rows — which changes the plan write model, excluded here twice over.
- **The plan write API, the revision model, and any workflow prompt.** Excluded by the
  work item; this capability is read-side only.
- **Containing case-variant or whitespace-variant tag forgeries.** AC-001.14 matches
  the exact literals case-sensitively; `<KANDEV-SYSTEM>` and `< kandev-system >` are NOT
  removed. Deliberate: the delimiter Kandev emits is the exact literal, `StripTags`
  already matches case-sensitively, and broadening the match would rewrite legitimate
  plan prose discussing the tag — as this file does. A follow-up needs a stated policy
  for that trade-off.
- **Bounding any continuation field other than `PlanSummary`.** `TaskDescription`,
  `Conversation`, `ToolSummary`, `RepositorySummary` and `FailureReason` keep their
  `bounded()` behaviour.
- **Changing either site's policy for a failed plan read.** They diverge today: the
  handover site ignores the error and injects no plan section, the dynamic site returns
  it and aborts the continuation package. Both are left as they are. Two details a
  follow-up needs: `GetTaskPlan` returns `(nil, nil)` for a missing plan and never
  surfaces `sql.ErrNoRows`, so `addDynamicPlan`'s `errors.Is(err, sql.ErrNoRows)` branch
  is unreachable; and the handover error path emits no log.
- **Making the budgets configurable.** Named constants; a settings surface, environment
  variable, or runtime flag would be speculative, with no requester.
- **Token counting.** The budget is in bytes, as `len` and `bounded()` measure,
  deliberately not calibrated per model or tokenizer.
- **Behaviour on input that is not valid UTF-8.** AC-001.8 guarantees rune safety for
  input that IS valid UTF-8 and deliberately says nothing about input that is not. Plan
  content reaches `task_plans` as JSON through the plan write API and is therefore
  expected to be valid UTF-8. Nothing fails if it is not — every cut lands on a line or
  section boundary and Go slicing is defined over any byte sequence — but the output is
  then not guaranteed to be valid UTF-8 either, because the input was not. A follow-up
  wanting that guarantee needs a validation or replacement policy at the plan write
  boundary, which is the write model this capability does not touch.
- **Summarising or rewriting plan content.** The reducer only selects and drops; it
  never asks a model to compact anything. Its output is always a subsequence of its
  input plus the two GENERATED literals — omission notice, and on the intra-section path
  the cut marker — and any separator byte before them. Containment only removes bytes,
  so it does not weaken this.

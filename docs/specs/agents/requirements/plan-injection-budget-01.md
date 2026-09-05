---
status: draft
system: agents
created: 2026-09-01
owners:
  - nova28
---

# Plan injection budget Requirements

## Overview

Kandev pastes a task's implementation plan into the prompt of a launching agent session
at two independent sites. Neither bounds the plan against a stated budget, and both
reduce it — when they reduce it at all — in a way that discards the newest part of the
running record without telling the agent anything was lost. This capability introduces
one shared, deterministic plan-bounding policy, applies it at both sites, and requires that any
reduction be declared in the injected text.

The agent system owns this contract: both consumers are agent-runtime session-launch
surfaces, and [dynamic agent routing](../system-design/dynamic-agent-routing-01.md)
already requires "a bounded continuation package" without defining the bound. The task
and workflow system keeps the plan artifact, its write API, and its revision model;
this capability only reads.

Observable behavior is defined here. The paired [system design](../system-design/plan-injection-budget-01.md)
records the components, control flow, implementation details, and rationale. Its
measurement and prior-art notes are in [system design Part 2](../system-design/plan-injection-budget-02.md).
The two requirements in this file cover the one plan-injection outcome.

## Terminology

- **Plan document / composed document:** the text prepared for injection by each
  surface. Handover keeps the current content composition. Dynamic continuation
  combines the title and content with its current whitespace handling. The budget
  applies to this plan text, not to the surrounding prompt frame or prefix.
- **Section:** a run of the plan document beginning at a line starting with `## ` and
  continuing to the line before the next such line, or to the end. Content before the
  first such line is the **preamble section**, and ONLY when there is such content: a
  document whose first line already begins with `## ` has NO preamble section and SHALL
  NOT be given a zero-length one, which would inflate `{total}` by one. Every other
  section begins at a `## ` line, so the preamble is the only section that could ever be
  empty. A document with no `## ` line is one.
- **Budget:** the maximum size in bytes of the reducer's output, including any
  omission notice, cut marker, and separator bytes.
- **Line:** a run of a section up to AND INCLUDING the next `\n`, or — for a final line
  carrying no terminator — up to the section's end. Both are COMPLETE lines, and a
  line's length counts its terminator when it has one. Cutting at a **line boundary**
  therefore leaves text ending with `\n` or at the section's end; the latter is normal,
  not a corner, the dynamic site's composed document being trimmed.
- **Omission notice:** the fixed template appended when, and only when, the reducer
  dropped content, carrying two independent facts:

  ```text
  [Kandev: plan reduced to fit the injection budget; {omitted} of {total} sections omitted{shortened}. Call get_task_plan_kandev for the full plan.]
  ```

  `{omitted}` and `{total}` are decimal integers. `{shortened}` is the literal
  `, and the retained section was shortened` when the intra-section path of AC-001.7
  ran, and empty otherwise.
- **Inline cut marker:** the fixed literal `[Kandev: section truncated here]`, emitted
  only on the intra-section path.
- **Containment:** the transformation of AC-001.14. Not part of the reducer.

## Measurement

The design's Appendix A records the read-only corpus measurement that informed the
budgets and section policy.

## Requirements

### REQ-AGENTS-PLAN-INJECTION-BUDGET-001: Bounded, deterministic plan injection

**Intent:** Bound what a launching session pays for plan context, keep the parts that
describe both the plan's framing and its current state, and never let the agent believe
a reduced plan is the whole plan.

**User story:** As an agent starting a new session on a task that already has a plan, I
want a bounded excerpt that tells me when it is an excerpt, so that I neither exhaust my
context on a document I can fetch nor act on a silently truncated record as complete.

#### Acceptance criteria

- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.1:** WHEN a plan document is at most the
  budget in bytes, THEN the reducer SHALL return it byte-identical and SHALL NOT append
  an omission notice. Equality with the budget is within budget. Byte-identity is
  relative to the document the reducer RECEIVES, which at the handover site is the
  composed document after AC-001.14 containment. AC-002.3 is the complete register of
  the respects in which injected bytes may differ from today.
  PRECEDENCE: this is the GENERAL case and it yields to the two special ones, which are
  evaluated first. AC-001.12 governs an empty or whitespace-only composed document — it
  injects nothing, even though it is trivially within budget, so byte-identity does NOT
  apply to it — and AC-001.13 governs a budget too small to emit anything at all. Where
  either applies it wins; everywhere else this criterion holds.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.2:** WHEN a plan document exceeds the budget,
  THEN the reducer's output SHALL be at most the budget in bytes, including the
  omission notice, inline cut marker, and any separator bytes. This bound SHALL hold
  for every input.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.3:** WHEN the reducer drops any content, THEN
  the output SHALL end with the omission notice of [Terminology](#terminology). Its two
  facts are reported independently. `{omitted}` SHALL be the number of whole sections
  not represented in the output AT ALL and `{total}` the document's section count, a
  section present but shortened by AC-001.7 NOT counting as omitted. Zero is therefore
  correct ONLY when the document has one section: when AC-001.7's path runs on a
  document of `{total}` sections it retains a shortened first section and drops the
  rest, so `{omitted}` SHALL be `{total} - 1`. The notice is tied to content having been
  dropped, never to a non-zero section count. This criterion yields to AC-001.13: when
  the reducer can emit no plan content at all it SHALL return nothing, the notice
  included, even though content was dropped. That is the ONLY case in which dropped
  content is not followed by a notice.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.4:** WHEN the reducer drops no content, THEN
  its output SHALL NOT contain an omission notice.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.5:** WHEN the reducer drops content, THEN it
  SHALL retain a contiguous run of whole sections from the start of the document and a
  contiguous run from the end, and SHALL drop only whole sections from the middle,
  except as permitted by AC-001.7. Either run MAY be empty; retaining only a tail run is
  expected when the first section does not fit.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.6:** WHEN the reducer selects which sections
  to retain, THEN it SHALL retain at most one contiguous run from the start and one
  contiguous run from the end of the document. The tail run SHALL be considered first,
  followed by alternating consideration of the two runs. A run SHALL close when its
  next section does not fit. Retained sections SHALL remain in document order, and the
  same input and budget SHALL always select the same sections.
  The output SHALL preserve the selected section text without rewriting it and SHALL
  not add separators between retained sections or end with a trailing newline.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.7:** WHEN the selection of AC-001.6 retains
  no whole section, THEN it SHALL retain leading complete lines from the first
  section, add the inline cut marker, and end with the omission notice. It SHALL cut
  only at line boundaries. If no complete line fits with the generated text, it SHALL
  return no plan text. It SHALL NOT shorten a section when any whole section fits.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.8:** The reducer's output SHALL be valid
  UTF-8 for every input that is valid UTF-8. A cut SHALL NOT split a rune.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.9:** WHEN the reducer is called twice with
  the same plan document and budget, THEN it SHALL return byte-identical output,
  depending only on those two inputs — not on the workflow step, session, task,
  agent, clock, map iteration order, or any heading's text.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.10:** WHEN the reducer is applied to its own
  output under the same budget, THEN the result SHALL be byte-identical to that
  output.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.11:** WHEN the plan document contains no
  `## ` line, THEN it SHALL be treated as one section; WHEN such a document also exceeds
  the budget it is governed by AC-001.7, one that fits returning unchanged under
  AC-001.1. WHEN
  it contains content before its first `## ` line, THEN that preamble SHALL be the
  first section, eligible for retention on the same terms as any other.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.12:** WHEN the plan is absent, or the site's
  composed document is empty or only whitespace, THEN no plan text and no omission
  notice SHALL be injected, and the launch SHALL proceed unchanged. Emptiness SHALL
  be judged after the surface applies containment, where containment applies. A title
  with whitespace-only content remains a title-only document on dynamic continuation;
  handover treats whitespace-only content as empty. These are the two named exceptions
  in AC-002.3.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.13:** WHEN the reducer cannot emit any plan
  content within the budget, THEN it SHALL return no plan text at all. It SHALL NOT
  return a notice or marker without plan text, and it SHALL NOT exceed the budget.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.14:** The injected plan text SHALL NOT be able
  to terminate or forge the enclosing `<kandev-system>` block. At handover, all
  occurrences of the exact `<kandev-system>` and `</kandev-system>` literals SHALL be
  removed, including literals exposed by earlier removals. Matching SHALL be
  case-sensitive and exact-literal; no other text SHALL change. Containment SHALL
  happen before reduction and only at handover. Dynamic continuation does not contain
  these literals because it does not place plan text in that block.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.15:** The reducer SHALL hold no mutable shared
  state that can change another call's output. Concurrent calls with the same input and
  budget SHALL produce the same result.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.16:** WHEN a plan is mutated between two
  session launches, THEN each launch SHALL reflect the plan snapshot used for that
  launch. A later mutation SHALL NOT change an already prepared injection.

### REQ-AGENTS-PLAN-INJECTION-BUDGET-002: Consistent plan injection across surfaces

**Intent:** Keep the bounded-plan contract consistent wherever Kandev injects plan
context, so one surface does not silently lose the other surface's protections.

**User story:** As an engineer changing plan context, I want both injection surfaces
to follow the same contract, so their behavior does not drift.

#### Acceptance criteria

- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.1:** The handover and dynamic continuation
  surfaces SHALL apply the same section retention, truncation, notice, and UTF-8
  rules from REQ-001. Handover SHALL also apply the containment rule in AC-001.14;
  dynamic continuation SHALL not apply containment.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.2:** An over-budget plan at either shipped
  surface SHALL produce output within that surface's budget. When plan text can fit,
  the output SHALL declare omitted content with the notice in the shared contract.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.3:** Each surface SHALL keep its current plan
  composition. Under budget, injected plan bytes SHALL remain unchanged except for
  these two handover cases: exact system-tag literals are removed, and whitespace-only
  content produces no plan section. Dynamic continuation SHALL still inject a title
  when its content is whitespace-only.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.4:** The dynamic plan excerpt SHALL fit within
  the continuation field's current size limit without a second truncation changing
  the reducer's output.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.5:** The handover budget SHALL be 12,000 bytes
  and the dynamic continuation budget SHALL be 4,000 bytes. Each budget SHALL apply
  to the plan document only, before the surrounding prompt frame or prefix.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-002.6:** WHEN plan content is reduced, the backend
  SHALL write an `Info` log with `site`, `task_id`, `plan_input_bytes`,
  `plan_output_bytes`, and `plan_sections_omitted`. The site value SHALL be
  `handover` or `dynamic_continuation`. The log SHALL contain no plan text. WHEN no
  reduction occurs, reduction fields SHALL be absent.

## Out of scope

- Selecting sections by relevance to the incoming workflow step.
- The plan write API, revision model, and workflow prompt.
- Containing case-variant or whitespace-variant tag forgeries.
- Bounding continuation fields other than the plan summary.
- Changing either surface's existing behavior when a plan read fails.
- Making the budgets configurable.
- Token counting or model-specific calibration.
- Defining behavior for input that is not valid UTF-8.
- Summarizing or rewriting plan content.

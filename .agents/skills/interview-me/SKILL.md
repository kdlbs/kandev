---
name: interview-me
description: Clarify a standalone idea or check assumptions before feature or fix planning. Use focused questions, stress-test intent, and map unresolved decisions for large uncertain initiatives.
---

# Interview Me

Run the assumption check before requirements, system designs, or implementation
plans. Reuse settled answers throughout the design package. An assumption check
does not require an interview when the material choices are already clear.

For a standalone interview or stress test, clarify intent without automatically
creating a design package. Skip this workflow for mechanical edits and pure
information requests.

## 1. Check assumptions

Read the request, relevant specifications, source, and tests. Separate:

- **Confirmed:** the user explicitly requested or previously settled it.
- **Verified:** source, tests, or documentation establish a fact. Keep its source
  reference and distinguish current behavior from intended behavior.
- **Unresolved:** a choice or missing fact can change behavior, scope, ownership,
  permissions, persistence, compatibility, or acceptance criteria.

State the intended outcome and material unresolved assumptions briefly. Do not
invent confidence percentages. Investigate facts that available tools can
resolve before asking the user. Evidence of current behavior is not user
agreement to preserve it.

Ask about consequential choices that evidence cannot settle. Choose routine,
reversible implementation details within the agreed scope. A request for speed
reduces questions; it does not make an unanswered material choice confirmed.

For a clear regression, use the active acceptance criterion without asking the
user to define the behavior again.

## 2. Ask questions in dependency order

Identify which decisions depend on other answers. Ask only questions whose
prerequisites are settled. Use the active harness's user-question tool and obey
its limits and waiting rules. Ask one to four independent questions per round,
within those limits. Without a question tool, ask one question at a time in chat
during a normal interactive session.

If this is an autopilot root or another non-interactive session with no question
tool, record each unresolved material choice as an assumption. Continue only
when the caller permits autonomous planning for that choice and use the most
conservative reversible option. Otherwise return the assumption as a blocker to
the caller. Never present an assumption as confirmed, and preserve the normal
chat fallback for interactive sessions.

Give each question concrete options, a recommended answer, and a short reason.
Align the question with the recommendation so agreement has one clear meaning.
Do not ask the user to locate code or supply facts you can investigate.

For example, first settle what "preserve task context" includes. Ask about
retention duration only after the user chooses persistent context.

Wait for answers before resolving dependent choices. After each round, update
the remaining questions. If an answer changes an earlier assumption, revisit
the affected choices and artifacts. Do not repeat settled questions without new
evidence or changed scope.

If the initiative has too many dependent unknowns to specify reliable work
orders, use [decision mapping](references/decision-mapping.md). Ordinary features
and clear fixes do not need a map.

## 3. Stress-test the answers

Use concrete scenarios to expose material gaps in success, failure, recovery,
and scope. Select scenarios relevant to the request; do not invent adjacent
features or an exhaustive questionnaire.

Challenge vague terms such as "robust" with an observable outcome. Compare domain
terms with the owning specifications and any existing glossary. If a term or
claimed behavior conflicts with those sources, name the conflict and resolve it.

Distinguish uncertainty that needs evidence from a choice that needs the user.
If a design question needs an experiment, define the question and observable
result first. Keep prototypes temporary and separate from production changes.
Record what the experiment proves and what remains undecided.

## 4. Capture decisions and continue

After each answer, preserve the choice and its reason in the current task notes.
Use the Kandev task plan when available, preserving user edits. Do not create a
separate intent document unless requested.

During authorized specification or planning work, put settled behavior in the
owning requirement and technical contracts in its system design. Use `/record`
for significant decisions that meet its ADR criteria. Preserve meaningful
alternatives and rationale there. Update an existing glossary when terminology
changes; do not introduce a parallel glossary by default.

Keep unresolved choices visibly unresolved. A recommendation, silence, or timeout
is not confirmation. If the user explicitly delegates a choice, record the
selected option as an agent decision made under that delegation.

When intent is clear and no material choice blocks the next design phase,
finish the interview. This includes success criteria, constraints, and scope.
Explicit answers and prior
instructions count as confirmation; do not require a second approval of their
restatement. If broad intent remains ambiguous, ask about the specific remaining
gap before continuing.

Summarize the settled intent and exclusions. If another skill called this check,
return to its current phase without restarting specification or planning.
For a direct planning request, continue through `/spec` and `/plan` to the
existing design-package handoff. For a standalone interview, return the clarified
intent. Neither path authorizes implementation or delegation.

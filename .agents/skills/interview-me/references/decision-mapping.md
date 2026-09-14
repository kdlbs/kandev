# Decision mapping

Use this mode for a large initiative whose dependent unknowns prevent reliable
specifications or work orders. Start with the desired outcome and scope. Keep
ordinary planning in the main interview workflow.

## Map only what is known

Use a section in the existing Kandev task plan when available. Otherwise use
working notes. For work that must survive across sessions without that tool,
use `docs/plans/<initiative>/discovery.md`. Read existing content before updates.

The map contains:

- **Destination:** the outcome this investigation must make possible.
- **Open decisions:** precise questions, dependencies, evidence needed, and status.
- **Settled decisions:** short summaries linked to their authoritative artifacts.
- **Not yet specified:** in-scope areas whose questions depend on earlier answers.
- **Out of scope:** excluded work and the reason for exclusion.

For each open decision, use a short name and record:

| Field | Content |
| --- | --- |
| Question | One decision or factual uncertainty to resolve |
| Depends on | Questions that must resolve first, or none |
| Method | Source investigation, external research, prototype, or user question |
| Status | Open, blocked, or resolved |
| Resolution | Answer, supporting evidence, and affected artifact links |

An unanswered but precise question belongs in open decisions, even when blocked.
Use "Not yet specified" only when the question itself is still unclear.
Do not turn vague areas into speculative implementation tasks.

## Resolve dependencies

Work on questions with settled prerequisites. Investigate facts in the primary
session. Ask the user about material choices through the interview workflow.
For a prototype, state the question, smallest experiment, and evidence needed
before creating temporary files. A prototype does not authorize production code.

After each resolution, revisit dependent questions. Add questions that are now
precise, update invalidated answers, and remove resolved uncertainty. If a
question is outside the destination, record its exclusion instead of expanding
the initiative.

Continue while useful investigation is possible within the authorized scope.
There is no one-decision-per-session limit. Do not create tracker issues,
persistent Kandev tasks, additional sessions, or subagents without explicit
authorization for those operations.

## Return to the design package

Finish discovery when no unresolved decision blocks the requirements, system
design, or reliable implementation order. Exclude remaining uncertainty from
the current scope explicitly, or keep the affected work blocked.

Move settled contracts into the owning requirements and system designs. Use
`/record` only when its ADR criteria apply. Replace detailed map resolutions
with summaries and links after those artifacts become authoritative.

Continue to `/plan` for implementation work orders. Each work order delivers
behavior; a discovery question resolves uncertainty. The map is neither an
implementation plan nor an additional approval checkpoint. Preserve the normal
design-package handoff before implementation.

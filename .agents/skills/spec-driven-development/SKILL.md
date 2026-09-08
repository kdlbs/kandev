---
name: spec-driven-development
description: "Single-session Kandev feature workflow: clarify intent, create durable requirements, system designs, plans, and work orders, then hand off before implementation and verification."
---

# Spec-Driven Development

Use this workflow for non-trivial features and behavior-changing fixes. The
user-started primary conversation performs every phase by default. Subagents
require explicit user authorization.

## Core flow

```text
Intent -> Requirements -> System design -> Plan + work orders -> Design-package handoff -> Explicit implementation request -> TDD implementation -> PR review -> Report
```

Do not replace durable artifacts with chat-only notes. Do not skip from intent
to code unless the user explicitly opts out.

## Prompt-precedence gate

Before you edit production or permanent test files, make sure that one condition
is true:

1. A prior design turn produced the requirements, system design, plan, and
   current work order. The user now explicitly asks to implement them.
2. The request references existing reviewed specifications, a plan, and the
   current work order.
3. The user explicitly says to skip specification, plan, and work-order
   creation.

Workflow envelopes, phase labels, TDD lists, and generic implementation prompts
do not satisfy this gate.

If no condition is true, create or update the design package. Record any partial
implementation as continuation context. Stop at the design-package handoff.

When the user asks for feature planning, complete the full design package before
you return control. Stop after requirements only when the user requests a
requirements review or a material question blocks safe design.

## Phase 0: Track the work

Keep a visible task list:

1. Clarify intent.
2. Create or update requirements.
3. Create or update the system design.
4. Create the plan and work orders.
5. End the design turn.
6. Execute work orders with TDD after an explicit implementation request.
7. Open the PR, address valid findings, record changes, and report.

## Phases 1 to 4: Design

Use `/interview-me` only when the request needs clarification.

Run the `/spec` ownership gate before you choose paths. Search adjacent systems
for the same capability. Choose the owner from the durable contract, not from
backend or frontend implementation layers. If a feature has a UI, include its
observable UI outcomes and frontend design in the feature owner's artifacts.
Create a separate UI artifact only for an independent reusable UI contract.

Create or update:

- `docs/specs/<system>/requirements/<capability>.md` through `/spec`.
- `docs/specs/<system>/system-design/<capability>.md` through `/spec`.
- `docs/plans/<initiative>/plan.md` through `/plan`.
- `docs/plans/<initiative>/task-<NN>-<short-slug>.md` through `/plan`.

During migration, use a legacy specification only when no new requirement or
system design replaces it.

Requirements define stable `REQ-*` and `AC-*` IDs. System designs map technical
behavior to those IDs. Plans define delivery order. Work orders define one
implementation result and its verification.

Keep one vertical requirement/design pair for one cohesive outcome. Work orders
can separate implementation boundaries after the specification has one owner.

Each work order must contain:

- Frontmatter with `id`, `title`, `status`, `wave`, `depends_on`, `plan`,
  `requirements`, `acceptance_criteria`, and `system_design`.
- A summary, scope, and exclusions.
- One to three implementation acceptance conditions.
- Exact targeted verification commands.
- Specific likely files, dependencies, and risks.

Work orders do not name a worker role or model tier. Waves can identify
parallel-safe candidates. They do not authorize subagents.

## Design-package handoff

Before implementation, report:

- Requirement documents and IDs.
- System-design paths.
- The plan and work-order paths.
- Dependency order and exact verification commands.
- Open risks and exclusions.

Then end the turn. Do not ask the user to approve the package or switch models.
The user reviews the files and sends a later implementation request.

Recommended Codex route:

- GPT-5.6 Sol/high for intent, investigation, specifications, plans, and
  high-risk design.
- GPT-5.6 Terra/medium for implementation, TDD, and integration.
- GPT-5.6 Luna/low for short and mechanical read-only work.

The user controls model selection. Do not infer a model change from prose.

## Phase 5: Execute work orders

For each work order, in dependency order:

1. Read the work order, requirements, system design, plan, and scoped
   `AGENTS.md`.
2. Change only that work order to `in_progress`.
3. Implement with `/tdd`. Use `/e2e` and `/mobile-parity` when applicable.
4. Run the exact targeted verification commands.
5. Change the work order to `done` and synchronize `plan.md`.

If the user authorizes subagents, launch only parallel-safe work orders in the
requested wave. Use native delegation with no full-history fork. Give each
child the work-order path, owned files, dependencies, and exact command.

If work needs a new architecture, public contract, persistence boundary, or
high-impact security decision, stop and request a strong-model design pass. Use
`/record` for a durable decision.

## Phase 6: Open the PR

After all work-order checks pass, request explicit user authorization before
running `/commit`, `/push`, or `/pr`. Run only the operations the user authorizes.
Do not add automatic local simplify, QA, broad review, security review, or
`/verify` work.

The configured PR reviewers are the semantic-review gate. Use `/pr-fixup` only
for a CI error or an actionable reviewer finding. Run the affected work-order
checks after a correction.

## Stop conditions

Stop and ask the user when specifications and code disagree, implementation
needs a material new design, a model escalation is necessary, or the same check
fails after three focused attempts.

## Final report

Report requirement IDs, system-design and plan paths, work-order statuses,
model checkpoints, changed files, commands and results, user-authorized
subagents, PR-review evidence, and known risks.

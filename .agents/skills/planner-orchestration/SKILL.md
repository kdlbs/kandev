---
name: planner-orchestration
description: Enforce Kandev's single-session, user-controlled model workflow for feature, fix, debug, review, verification, and delivery work.
---

# Single-Session Orchestration

The user-started primary conversation owns durable artifacts, integration
judgment, and user communication. Platform-provided explorers and other
predefined subagents may continue to serve the harness's normal investigation
workflow. This skill governs planned implementation delegation, not the
platform's general exploration behavior.

## Model Checkpoints

The user, not the harness, selects the model. Keep each phase in the primary
conversation so the active model, transcript, and costs are visible in one place.

1. **Design checkpoint — strong model.** Use the user's strong model for
   clarification, codebase investigation, specs, plans, task decomposition, and
   high-risk design decisions. Default Codex guidance is GPT-5.6 Sol/high.
2. **Design-package handoff.** Once the spec, plan, and task files are ready,
   summarize their paths and end the turn. Do not call
   `ask_user_question_kandev` (or an equivalent approval prompt) to ask the
   user to approve the package or switch models. The user reviews the files,
   switches the main session if desired, and sends a later explicit
   implementation request. The files may remain `draft`/`pending`; do not wait
   for a separate approval marker.
3. **Execution checkpoint.** After that explicit request, read the
   task file, mark it `in_progress`, implement with `/tdd`, run its exact
   targeted checks, and mark it `done`. Work sequentially through the plan by
   default. The user, not the harness, chooses the active implementation model.
4. **Escalation checkpoint.** Stop and ask the user to switch back to a stronger
   model before an architectural redesign, a new public contract, a migration or
   persistence boundary, or a high-impact security decision. Record a durable
   decision when the `/record` trigger applies.

Luna/low is appropriate only for clearly mechanical, read-only work such as
short status summaries or simple command-output interpretation. It is not the
default implementation or test model. The active agent must never claim a model
change occurred based on self-identification; use runtime model-usage metadata
when such confirmation is needed.

## Task-File Workflow

Feature work still follows `/spec`, `/plan`, and `/spec-driven-development`:

- Store the accepted spec in `docs/specs/<slug>/spec.md`.
- Store `plan.md` and independently actionable sibling task files in
  `docs/plans/<slug>/`.
- Use `pending`, `in_progress`, and `done` task-file status as the durable
  execution record. The primary agent updates both the current task and the
  plan's status after each completed task.
- Keep task files small enough that the same conversation can resume from their
  acceptance criteria, owned files, and exact verification command after a user
  switches model.

Task files are a model-switch handoff and, if the user explicitly asks for
subagents, a compact work packet. They must include intent, dependencies, owned
files, acceptance criteria, verification, and risks, but must not name an agent
role or model tier.

## User-Authorized Subagents

Plans may label dependency waves and parallel-safe candidates. That is planning
information only: execute sequentially unless the user explicitly asks to use
subagents after selecting the implementation model.

When the user authorizes subagents:

- Use the platform's native delegation tool, never Kandev task/session MCP APIs.
- Do not recreate project custom-agent files or pin a different worker model.
  The child must use the active model the user selected in the primary session.
- Use `fork_turns: "none"` (or the platform equivalent), not a full-history
  fork. Put the task-file path, owned files, acceptance criteria, exact command,
  and dependencies in the initial prompt.
- Launch only tasks explicitly marked parallel-safe with disjoint files and no
  shared schema, migration, generated contract, lockfile, or package config.
- Tell each child not to spawn further children. Update shared `plan.md` status
  serially in the primary session.
- Confirm model routing from runtime usage metadata, not a model's prose. If it
  does not show the user-selected model, stop the delegation and report it.

If the user does not explicitly authorize delegation, do not infer permission
from a plan's waves or parallelism labels.

The read-only `pr-poller` is a delivery exception, not an implementation
worker. Launch it only after the user explicitly asks to wait for or monitor PR
updates; it reports status to the primary conversation and never remediates,
comments, or spawns children.

## Task-Driven Validation And PR Review

Each task file owns its TDD requirement and exact unit, integration, or E2E
commands. Its completed status and recorded command results are the pre-PR
evidence; do not add a second generic validation pass here.

Do not automatically run `/simplify`, `/qa`, `/code-review`, security review,
or broad `/verify` before opening a PR. Those duplicate the task validation and
the two configured PR AI reviewers. Run them only when the user explicitly asks
or an actionable PR/CI finding requires a focused remediation.

After the PR opens, the two configured AI reviewers are the semantic-review
gate. Use `/pr-fixup` only to address a CI failure or actionable reviewer
finding. A remediation reruns its relevant task-defined checks, not a broad
local suite unless explicitly requested.
Treat the OpenCode App as trusted semantic evidence only when `trusted_producer=true` confirms its dedicated producer provenance.

## Guardrails

- Do not treat a plan wave as authorization to launch implementation workers;
  that decision remains with the user. This does not restrict platform-provided
  explorers or other harness-managed investigation agents.
- Do not use Kandev MCP task/session APIs as a worker mechanism. Use them only
  when the user explicitly asks to manage persistent Kandev tasks or sessions.
- Do not create worktrees solely to parallelize plan tasks unless the user has
  explicitly authorized subagents for parallel-safe tasks.
- Do not continue from the design-package handoff automatically or treat
  artifact creation as implementation authorization. Wait for a later explicit
  implementation request; the user controls any model switch between turns.
- Do not replace durable specs, plans, task files, tests, or verification with
  chat-only summaries.

# Workflows

Kandev ships with four workflow templates. Each defines a sequence of steps with automated transitions - agents start, stop, and move between steps based on events like entering a step, sending a message, or an agent completing its turn.

You can use these as-is or **create custom workflows** tailored to you or your team.

## Default Workflows

### Kanban

Classic kanban board with automated agent execution. Good for straightforward tasks where you want to assign work, let the agent run, and review the result.

**Steps:** Backlog → In Progress → Review → Done

| Step | What happens |
|:------:|-------------|
| **Backlog** | Backlog of tasks not yet started. Sending a message moves the task to In Progress. |
| **In Progress** | Agent starts the work automatically. When it completes, the task moves to Review. |
| **Review** | You review the agent's work. Sending a message moves it back to In Progress for another iteration. |
| **Done** | Final state. Sending a message reopens the task in In Progress. |

**When to use it:**
- Bug fixes with clear reproduction steps
- Small, well-scoped features
- Chores and refactoring tasks
- Any task where you want a simple assign → run → review loop

---

### Plan & Build

Two-phase workflow where the agent first creates a plan for your review, then implements it. The plan is saved as a structured document you can edit before the agent proceeds to implementation.

**Steps:** Todo → Plan → Implementation → Done

| Step | What happens |
|:------:|-------------|
| **Todo** | Tasks ready to be planned. |
| **Plan** | Agent analyzes the task and creates a detailed implementation plan - requirements, files to modify, step-by-step approach, risks. Supports mermaid diagrams. The plan is saved via MCP tool and the agent stops for your review. You can edit the plan in the UI before moving forward. |
| **Implementation** | Agent retrieves the plan (including your edits), acknowledges modifications, and implements step-by-step. Moves to Done on completion. |
| **Done** | Final state. |

**When to use it:**
- Features that benefit from upfront design
- Tasks where you want to steer the approach before code is written
- Larger changes spanning multiple files
- When working with less familiar codebases where you want to validate the agent's understanding first

---

### Architecture

Focused on design and architecture. The agent creates technical designs for you to review - no implementation happens in this workflow. Useful for capturing architectural decisions before any code is written.

**Steps:** Ideas → Planning → Review → Approved

| Step | What happens |
|:------:|-------------|
| **Ideas** | Backlog of architectural ideas and proposals. |
| **Planning** | Agent analyzes the task, asks clarifying questions, and produces an architectural design with mermaid diagrams. Saves the design and stops for your review. |
| **Review** | You review the design. Sending a message moves it back to Planning for revisions. |
| **Approved** | Design is accepted. Ready for implementation (in a separate task/workflow). |

**When to use it:**
- System design and technical RFCs
- Evaluating approaches before committing to implementation
- Cross-team architectural proposals
- Breaking down large projects into implementable pieces

---

### PR Review

Track pull requests through automated code review. The agent reviews changed files and produces structured findings.

**Steps:** Waiting → Review → Done

| Step | What happens |
|:------:|-------------|
| **Waiting** | PR queue. Sending a message starts the review process. |
| **Review** | Agent reviews the changed files in the git worktree. If there are uncommitted changes, it reviews those; otherwise, it reviews commits that diverged from the main branch. Findings are organized into four categories: **BUG**, **IMPROVEMENT**, **NITPICK**, **PERFORMANCE** - each with file:line references. |
| **Done** | Review complete. Sending a message moves it back to Review for another pass. |

**When to use it:**
- Automated first-pass code review before human review
- Catching bugs, performance issues, and style problems early
- Reviewing agent-generated PRs from other workflows
- Supplementing human review on large changesets

---

## Tips

### Repository-Level Agent Configuration

Each repository should maintain its own agent configuration - `CLAUDE.md`, `AGENTS.md`, custom skills, MCP servers, and any agent-specific instructions. This keeps agent behavior consistent with the codebase it's working on, regardless of who triggers the task or which workflow runs it.

When an agent is assigned a task in a repository, it picks up that repository's configuration automatically. Coding standards, project context, and tooling constraints travel with the code, not with the platform.

### Cross-Repository Workflows

Workflow pipelines often span multiple repositories - a backend change triggers a client update, an infrastructure change follows a service deployment. Each step in the pipeline runs an agent inside a specific repository, and that agent should use the repository's own AI harness (prompts, rules, skills) rather than a shared global config.

This keeps each agent grounded in the right context. A backend agent follows backend conventions, a frontend agent follows frontend conventions, even when they're part of the same pipeline.

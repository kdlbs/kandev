# Office Onboarding Analysis

## Summary

The current kandev onboarding is informational (shows what's available) but doesn't create anything. Users must manually create agents, tasks, and projects after completing the wizard. The onboarding state is stored in localStorage (lost on clear/device switch).

## Current kandev onboarding

- 4 steps: Agents (discover), Executors (informational), Workflows (informational), Commands (informational)
- No agent auto-creation, no starter task, no project
- Completion stored in localStorage, not backend
- No redirect guard for new workspaces
- Empty office dashboard shows zeros everywhere

## What's needed

### CEO agent is not optional
Every office workspace needs a CEO agent to function. Auto-creating it with sensible defaults reduces friction.

### Recommended flow (6 steps)

1. **Welcome**: "We'll set up your workspace, create a CEO agent, and optionally give it work."
2. **Workspace setup**: name (default "Default Workspace"), task prefix (default "KAN")
3. **Agent discovery**: show available agent CLIs (existing step, simplified)
4. **Create CEO**: pre-filled form (name="CEO", role=ceo, executor from step 3). Agent profile selector.
5. **First task** (optional): title + description for the CEO's first assignment
6. **Review & Launch**: summary of what will be created, "Create & Launch" button

### Backend requirements

- `onboarding_state` tracking per workspace (completed, ceo_agent_id, first_task_id)
- `GET/POST /api/workspaces/:id/onboarding-state` endpoints
- Redirect guard: new workspaces redirect to onboarding until completed
- `CreateCEOAgentWithDefaults()` service method
- CEO instructions bundle (AGENTS.md with delegation rules)

### What gets auto-created

On "Create & Launch":
1. Workspace (if new)
2. CEO agent with default permissions + instructions
3. "Onboarding" project (optional)
4. First task assigned to CEO (optional)
5. Onboarding marked complete

### Edge cases
- Returning users: query backend state, skip if completed
- Multiple workspaces: per-workspace onboarding state
- Users who skip: mark as "skipped", show inline CTA on agents page
- Shared workspaces: one user completing suppresses for others

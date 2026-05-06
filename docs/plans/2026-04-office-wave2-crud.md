# Office Wave 2: Core CRUD (Agents, Skills, Projects)

**Date:** 2026-04-26
**Status:** proposed
**Specs:** `office-agents`, `office-skills`, `office-projects`
**UI Reference:** `docs/plans/2026-04-office-ui-reference.md` (agent detail, skills page, new agent form, project list)
**Depends on:** Wave 1 (tables, routing, API stubs)

## Problem

Wave 1 creates the shell. Wave 2 fills in the CRUD for the three foundational entities: agent instances, skills, and projects. These are independent of each other and can be built in parallel.

## Scope

### 2A: Agent Instances (parallelizable)

**Backend** (`internal/office/`):
- Repository: full CRUD for `office_agent_instances`
  - `Create(ctx, instance)` -- validate unique name per workspace, set defaults by role
  - `Get(ctx, id)`, `List(ctx, workspaceID, filters)`, `Update(ctx, id, patch)`, `Delete(ctx, id)`
  - `ListByReportsTo(ctx, parentID)` -- for org tree
  - `UpdateStatus(ctx, id, status, pauseReason)` -- status transitions
- Service: business logic
  - Default permissions by role (ceo, worker, specialist, assistant)
  - Validate `reports_to` exists and is in same workspace
  - Validate `agent_profile_id` exists (via agent settings store)
  - At most one CEO per workspace
  - Emit `office.agent.created/updated/status_changed` events
- Controller: DTOs, validation
- Handlers: HTTP routes wired to controller

**Frontend:**
- `/office/agents` page: card grid of agent instances
  - Card: icon, name, role badge, status dot, budget gauge, skill badges, current task
  - "+" button opens create dialog
  - Click card navigates to `/office/agents/[id]`
- `/office/agents/[id]` page: tabbed detail view
  - Overview tab: name, role, status, reports_to, budget, permissions (editable)
  - Skills tab: assigned skills with toggle (read from skills list)
  - Runs tab: placeholder (populated in Wave 3/4)
  - Memory tab: placeholder (populated in Wave 7)
  - Channels tab: placeholder (populated in Wave 7)
- Create agent dialog: name, role, profile selector, reports_to, budget, skills
- Sidebar agents section: compact list with status dots, click to navigate
- Store actions: `setAgentInstances`, `addAgentInstance`, `updateAgentInstance`, `removeAgentInstance`
- API client: `listAgentInstances`, `createAgentInstance`, `getAgentInstance`, `updateAgentInstance`, `deleteAgentInstance`

**Tests:**
- Backend: CRUD repository tests, service validation tests (unique name, single CEO, valid reports_to)
- Frontend: store slice tests, API client type tests

### 2B: Skill Registry (parallelizable)

**Backend** (`internal/office/`):
- Repository: full CRUD for `office_skills`
  - `Create(ctx, skill)` -- auto-generate slug from name
  - `Get(ctx, id)`, `GetBySlug(ctx, workspaceID, slug)`, `List(ctx, workspaceID)`
  - `Update(ctx, id, patch)`, `Delete(ctx, id)`
  - `ListByAgentInstance(ctx, instanceID)` -- join via agent's desired_skills JSON
- Service: business logic
  - Validate unique slug per workspace
  - For `source_type=git`: clone repo, extract SKILL.md, build file_inventory
  - For `source_type=local_path`: validate path exists, build file_inventory
  - For `source_type=inline`: store content in DB
  - Emit `office.skill.created/updated` events
- Skill materialization (for session injection):
  - `MaterializeSkills(ctx, skillIDs) -> []SkillDir` -- returns on-disk paths
  - Inline skills: write to temp dir under workspace skill cache
  - Git skills: use cached clone
  - Local path skills: return path directly
  - `InjectSkills(worktreePath, agentType, skillDirs)` -- write skill dirs to `<worktree>/.claude/skills/kandev-<slug>/` (Claude) or `<worktree>/.agents/skills/kandev-<slug>/` (others)
  - `EnsureExcludePattern(worktreePath)` -- add `kandev-*` patterns to `.git/info/exclude`
  - No cleanup needed: injected skills are removed automatically with the worktree

**Frontend:**
- `/office/company/skills` page: skill list table
  - Columns: name, slug, source type, description, used by (agent count)
  - "Add Skill" button opens create dialog/page
  - Click row to edit
- Skill create/edit: name, description, source type selector
  - Inline: markdown editor for SKILL.md content
  - Git: URL input + branch/tag
  - Local path: path input
  - File inventory display (read-only)
- Store actions: `setSkills`, `addSkill`, `updateSkill`, `removeSkill`
- API client: `listSkills`, `createSkill`, `getSkill`, `updateSkill`, `deleteSkill`

**Tests:**
- Backend: CRUD tests, slug generation, materialization to temp dir, CWD injection and exclude pattern
- Frontend: store tests

### 2C: Projects with Multi-Repo (parallelizable)

**Backend** (`internal/office/`):
- Repository: full CRUD for `office_projects`
  - `Create(ctx, project)` -- repositories as JSON array
  - `Get(ctx, id)`, `List(ctx, workspaceID)`, `Update(ctx, id, patch)`, `Delete(ctx, id)`
  - `GetTaskCounts(ctx, projectID)` -- aggregate task stats by status
- Service: business logic
  - Validate repository URLs/paths
  - Emit `office.project.created/updated` events
- Task-project relationship:
  - Extend task service: `UpdateTaskProject(ctx, taskID, projectID)`
  - Extend task list queries: filter by `project_id`

**Frontend:**
- `/office/projects` page: project list
  - Cards: name, color dot, status, repo count, task counts, progress bar, lead agent
  - "+" button opens create dialog
- `/office/projects/[id]` page: project detail
  - Header: name, description, status, repositories list
  - Task list (reuse issues list component, filtered to project)
  - Budget section (placeholder until Wave 5)
- Create project dialog: name, description, color, repositories (add/remove list), lead agent
- Sidebar projects section: expandable list with color dots
- Store actions: `setProjects`, `addProject`, `updateProject`, `removeProject`
- API client: `listProjects`, `createProject`, `getProject`, `updateProject`, `deleteProject`

**Tests:**
- Backend: CRUD tests, task count aggregation, task-project assignment
- Frontend: store tests

## Verification

1. `make -C apps/backend test` passes
2. `cd apps && pnpm --filter @kandev/web typecheck` passes
3. Can create/edit/delete agent instances via UI
4. Can create/edit/delete skills via UI (inline source)
5. Can create/edit/delete projects with multiple repos via UI
6. Sidebar shows agents and projects lists
7. Agent detail page shows tabs (Overview + Skills functional, others placeholder)

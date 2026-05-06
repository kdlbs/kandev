# Office Wave 3: Task System & Issues UI

**Date:** 2026-04-26
**Status:** proposed
**Specs:** `office-overview` (issues list, task detail, new issue dialog)
**UI Reference:** `docs/plans/2026-04-office-ui-reference.md` (issues list, task detail simple/advanced, new issue dialog, properties panel, chat thread)
**Depends on:** Wave 2 (agents, skills, projects exist for assignment)

## Problem

The issues list, task detail pages (simple + advanced mode), and new issue dialog are the core user-facing UI for office. This wave extends the existing task model and builds the issue tracker interface.

## Scope

### 3A: Task Model Extensions (backend, do first)

**Note:** DB schema changes (ALTER TABLE, new tables) are done in Wave 1. This wave adds the Go structs, service logic, and query extensions.

**Extend existing task model** (`internal/task/models/models.go`):
- Add fields to Task struct: `AssigneeAgentInstanceID`, `Origin`, `ProjectID`, `RequiresApproval`, `ExecutionPolicy`, `ExecutionState`, `Labels`, `Identifier`
- `Origin` enum: `manual`, `agent_created`, `routine`
- `ExecutionPolicy` struct: stages array with type (review/approval), participants, approvals_needed
- `ExecutionState` struct: current stage index, participant responses, status
- `WorkflowID` and `WorkflowStepID` remain non-nullable -- office tasks use the system office workflow

**Extend task repository** (`internal/task/repository/sqlite/`):
- Query extensions: filter by `project_id`, `assignee_agent_instance_id`, `origin`, `status`, `labels`
- List with hierarchical tree: query parent/child relationships via `parent_id`, build tree structure
- Blocker queries: `GetBlockers(taskID)`, `GetBlocking(taskID)`, `AddBlocker(taskID, blockerID)`, `RemoveBlocker(taskID, blockerID)`, `CheckCircularDependency(taskID, blockerID)`
- Comment CRUD: `CreateComment`, `ListComments(taskID)`, `GetComment(id)`
- Include new fields in all CRUD operations
- Office status changes implemented as MoveTask to the corresponding office workflow step (e.g. status=in_review -> move to "In Review" step)

**Extend task service** (`internal/task/service/`):
- `CreateTask` for office: set workflow_id to workspace's office_workflow_id, workflow_step_id to "Todo" step (no null workflow_id)
- `CreateTask`: auto-assign `identifier` from workspace sequence counter (atomic increment)
- `UpdateTask` handles assignee changes -> emit events for wakeup queue
- `GetTaskTree(ctx, workspaceID, filters)` -> returns flat list with parent_id for frontend tree building
- Status transitions: when office task reaches `done` with execution_policy, enter review/approval flow
- Blocker management: add/remove blockers, check resolution on task completion, fire `task_blockers_resolved` events
- Comment creation: validate author, set source, trigger `task_comment` wakeup for assignee

**Ensure existing flows still work:**
- Non-office tasks (kanban) continue to require workflow_id
- Existing task creation, move, workflow transitions unaffected
- Kanban board only shows tasks with workflow_id (office tasks excluded from kanban view)
- `/t/[taskId]` detail page still works for all tasks

**Tests:**
- Create office task -> gets system office workflow_id + "Todo" step automatically
- Create kanban task -> still requires explicit workflow_id (backwards compatible)
- CRUD with new fields
- Tree query tests (parent/child nesting, 3 levels deep)
- Blocker: add, remove, circular detection, resolution check
- Comment: create, list, wakeup trigger
- Identifier generation: sequential, unique per workspace, only for office tasks (null for existing kanban tasks)
- Execution policy validation (stages, participants)
- Status transitions via MoveTask: todo -> in_progress -> in_review -> done (moves between office workflow steps)

### 3B: Issues List Page (frontend, parallelizable with 3C)

**`/office/issues/page.tsx`**:
- Hierarchical tree view (default): parent tasks expandable, child tasks indented
- Board view: kanban columns by status (backlog, todo, in_progress, in_review, done, cancelled)
- Toolbar: `[+ New Issue] [Search] | [List/Board] [Nesting] [Columns] [Filters] [Sort] [Group]`

**Components to build:**
- `OfficeIssuesList`: main list component
  - View mode state: `list` | `board`
  - Nesting toggle (list mode only)
  - Progressive rendering for large lists
- `OfficeIssueRow`: single row
  - Status icon (colored dot), identifier (KAN-1), title, assignee avatar, timestamp
  - Indent level for nested children
  - Collapse/expand toggle for parent tasks
- `OfficeIssueBoard`: board view
  - Columns by status, drag-drop between columns
  - Reuse shadcn components
- `OfficeIssueToolbar`: filter/sort/group controls
  - Filters popover: status, priority, assignee (agent instances), project, labels
  - Sort popover: status, priority, title, created, updated (asc/desc)
  - Group popover: status, priority, assignee, project, parent, none
  - Column picker: status, id, assignee, project, updated
- `OfficeIssueFilters`: filter state management
  - Persist to localStorage per workspace

**API calls:**
- `GET /api/v1/office/workspaces/:wsId/issues` (extends existing task list with office filters)

**Store:**
- `office.issues`: `{ items: Task[], filters: FilterState, viewMode, sortField, sortDir, groupBy, nestingEnabled }`

**Tests:**
- Filter state management
- Tree building from flat list with parent_id
- View mode toggle

### 3C: Task Detail Pages (frontend, parallelizable with 3B)

**Simple mode** (`/office/issues/[id]/page.tsx`):

Layout:
```
| Breadcrumb (Issues > Parent > Task)                              |
| [Status icon] KAN-3 [Project badge] [Copy] [Menu]               |
|                                                                  |
| Title (editable)                                |  Properties    |
| Description (markdown, editable)                |  Panel         |
|                                                 |  (right)       |
| [+ New Sub-Issue] [Upload] [+ New doc]          |                |
|                                                 |                |
| [Chat] [Activity] tabs                          |                |
| Chat thread / Activity timeline                 |                |
|                                                 |                |
| Sub-issues section (reuse IssuesList)           |                |
|--------------------------------------------------------------- --|
```

**Components:**
- `OfficeTaskDetail`: main layout, simple/advanced toggle
- `OfficeTaskProperties`: right sidebar panel
  - Status dropdown, Priority dropdown, Labels multi-select
  - Assignee picker (agent instances + user)
  - Project picker
  - Parent link, Blocked by multi-select, Blocking (read-only)
  - Sub-issues list with "+ Add sub-issue"
  - Reviewers multi-select, Approvers multi-select
  - Created by, timestamps
- `OfficeTaskChat`: comment thread
  - Agent run transcripts with collapsible tool call details
  - User/agent comments
  - Input box for posting comments
- `OfficeTaskActivity`: timeline of status changes, assignments, approvals

**Advanced mode** (`?mode=advanced`):
- Dockview layout within office chrome (sidebar + topbar stay)
- Fixed layout: chat panel, terminal, plan panel | files, changes (right sidebar)
- Reuse existing dockview panel components from kandev task detail
- Auto-launch ACP session on enter (start/resume, no prompt = no tokens)
- Toggle button to switch back to simple mode

**Key implementation notes:**
- Advanced mode reuses `DockviewReact` with a fixed layout (no presets)
- Import existing panel components: `ChatPanel`, `TerminalPanel`, `PlanPanel`, `FilesPanel`, `ChangesPanel`
- Session management: call existing `StartTask`/`ResumeSession` orchestrator endpoints
- ACP session started without initial prompt (idle state)

**Tests:**
- Properties panel renders with correct fields
- Simple/advanced mode toggle
- Chat thread renders messages

### 3D: New Issue Dialog (frontend, parallelizable)

**Components:**
- `OfficeNewIssueDialog`: modal
  - Title (auto-expanding textarea)
  - Quick selector row: "For [Assignee] in [Project]"
  - Three-dot menu: toggle Reviewer/Approver rows
  - Description markdown editor
  - Bottom bar: Status chip, Priority chip, Upload, more
  - Footer: Discard Draft | Create Issue
  - Draft auto-save to localStorage (debounced)
  - Sub-issue badge when creating from parent context
- Reuse shadcn components: Dialog, Select, Button, Badge, Textarea

**API calls:**
- `POST /api/v1/tasks` (existing endpoint, extended with new fields)

**Tests:**
- Dialog renders, fields populate
- Draft persistence to localStorage
- Create fires API call with correct fields

## Verification

1. `make -C apps/backend test` passes
2. `cd apps && pnpm --filter @kandev/web typecheck` passes
3. Issues list shows tasks with hierarchical nesting
4. Can switch between list and board views
5. Filters, sort, and group work
6. Task detail simple mode shows description, properties, chat
7. Task detail advanced mode shows dockview layout
8. New issue dialog creates tasks with assignee, project, reviewers
9. Existing task functionality (kanban board, `/t/[taskId]`) still works

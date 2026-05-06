# Office E2E Test Plan

## Setup

- All tests run against `KANDEV_HOME_DIR=/tmp/kandev-e2e-<run>` (isolated, fresh DB per suite)
- Backend started via fixture with mock agent configured
- Mock agent (`cmd/mock-agent`) simulates ACP protocol (tool calls, permission requests, clarifications)
- Playwright config: `apps/web/e2e/playwright.config.ts`
- Existing patterns: `apps/web/e2e/tests/` (25+ specs, fixtures in `e2e/fixtures/`)

## Test Suites

### 1. Onboarding (`office/onboarding.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Fresh install → redirect to /office/setup | No workspace → 307 redirect |
| Wizard step 1: workspace name + prefix | Input renders, validation (empty name blocked) |
| Wizard step 2: CEO agent config | Agent name, profile selector, executor preference |
| Wizard step 3: first task (optional) | Task title/description, Skip button works |
| Wizard step 4: review + Create & Launch | Summary correct, submit creates workspace + agent + task |
| Post-onboarding → dashboard | Redirect to /office, dashboard shows 1 agent, 1 task |
| FS import prompt (shared config) | Pre-populate FS config → setup shows import prompt → Import & Continue |
| Start Fresh bypasses import | Import prompt → Start Fresh → wizard shows |

### 2. Agents (`office/agents.spec.ts`)

| Test | What it validates |
|------|-------------------|
| List agents page | Shows CEO after onboarding, correct role badge |
| Agent detail page | Overview tab with status, permissions, profile |
| Create agent | Dialog → fill fields → agent appears in list |
| Update agent | Edit name/role/budget → changes persist |
| Delete agent | Confirm dialog → agent removed |
| Agent status transitions | Pause → paused badge, Resume → idle badge |
| Agent instructions tab | List bundled instructions (CEO/worker AGENTS.md) |
| Create/edit/delete instruction | CRUD on custom instruction file |
| Agent memory tab | Write memory entry → list → read → delete |
| Agent skills tab | Shows assigned skills list |

### 3. Tasks & Issues (`office/tasks.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Tasks list page | Shows task created during onboarding |
| Task detail page | Title, description, status, assignee, project |
| Create task via "New Task" button | Dialog → fill → task appears with identifier (QA-N) |
| Task with parent (subtask) | Create with parent_id → shows parent badge |
| Task status icon | Matches task state (TODO/IN_PROGRESS/COMPLETED) |
| Issue filters | Filter by status, assignee, project |
| Task search | FTS5 search returns matching tasks |

### 4. Labels (`office/labels.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Add label to task | POST → label created with auto-color |
| Add duplicate label | Idempotent, no error |
| Remove label | DELETE → label removed from task |
| List labels on task | GET → returns label objects with name + color |
| Workspace label catalog | List all labels, correct colors |
| Update label color | PATCH → color changes |
| Delete label | Cascades removal from all tasks |
| Labels shown on task card | Issue list shows colored badge pills |
| Labels shown in task detail | Detail view shows labels with add/remove |

### 5. Documents (`office/documents.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Create document on task | PUT → document created with key, type, content |
| Read document | GET → returns latest content |
| Update document (revision) | PUT again → revision number increments |
| List documents | GET → returns all docs for task |
| Revert document | Restore prior revision → new revision created |
| Delete document | DELETE → document and revisions removed |
| Plan backward compat | Existing plan API → maps to key=plan document |
| Upload attachment | POST multipart → file stored on disk, metadata in DB |
| Download attachment | GET → streams file |
| Documents section in UI | Task detail shows Documents with type badges |

### 6. Execution Stages (`office/execution-stages.spec.ts`)

Mock agent scenarios use `kandev task update --status <done|in_progress> --comment "..."` to signal stage outcomes. No kanban moves are needed.

| Test | What it validates |
|------|-------------------|
| Task with work stage | Create with policy → assignee woken |
| Work → review transition | Assignee calls `task update --status done --comment "..."` → reviewers woken |
| Review approval | Reviewer calls `task update --status done --comment "LGTM"` → advances to next stage |
| Review rejection → rework | Reviewer calls `task update --status in_progress --comment "Needs fix"` → assignee re-woken with feedback |
| Re-entry after rework | Assignee fixes, calls `task update --status done` → re-enters same review stage |
| Approval stage (human) | Human approves via inbox → advances |
| Ship stage | After approval → assignee woken, calls `task update --status done --comment "PR created"` |
| Mandatory comment | `task update --status done` with no comment → error returned |
| Checkout lock | Second agent cannot update status while first holds lock |
| Stage progress bar UI | Task detail shows stage pills with correct highlighting |
| No policy → simple flow | Task without policy works as before |
| Full pipeline end-to-end | work → review → approval → ship → complete |

### 7. Skills (`office/skills.spec.ts`)

| Test | What it validates |
|------|-------------------|
| List skills (empty) | Workspace with no skills shows empty |
| Create skill | POST with name, slug, content → skill created |
| Import skill from path | Import from local directory |
| Get skill | Fetch by ID returns full content |
| Update skill | PATCH content → version updated |
| Delete skill | Removed from catalog |
| Assign skill to agent | Set desired_skills on agent → skill linked |
| Skill injection on agent wake | Mock agent wakes → verify skills written to worktree (`.claude/skills/kandev-*/` for Claude, `.agents/skills/kandev-*/` for others) |
| Bundled skills present | kandev-protocol + memory skills exist |
| Skills page UI | List, create, edit, delete through UI |

### 8. Config Sync (`office/config-sync.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Export config (JSON) | GET → returns ConfigBundle with agents, skills, projects |
| Export config (ZIP) | GET → downloads ZIP with YAML files |
| Incoming diff (FS → DB) | Modify YAML on disk → diff shows changes |
| Apply incoming sync | Apply FS changes to DB → entities updated |
| Outgoing diff (DB → FS) | Modify via API → diff shows DB changes |
| Apply outgoing sync | Write DB state to FS → YAML files updated |
| Config sync UI | Settings page shows sync panes with diffs |
| Export preview UI | File tree with YAML preview |
| Round-trip test | Export → modify YAML → import → verify changes |

### 9. Routines (`office/routines.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Create routine | POST with name, task template, cron → routine created |
| List routines | Page shows routine with schedule |
| Create trigger (cron) | Cron trigger created, next fire time calculated |
| Create trigger (webhook) | Webhook URL generated with HMAC secret |
| Fire webhook trigger | POST to webhook URL → task created |
| Manual run | Fire routine manually → task created |
| Routine runs history | Shows past runs with status/duration |
| Delete routine | Cascades trigger + run deletion |
| Routines page UI | List, create, run, delete through UI |

### 10. Costs & Budgets (`office/costs.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Record cost event | Agent session → cost event recorded |
| Cost summary | Aggregate total spend for workspace |
| Cost breakdown by agent | Group costs per agent |
| Cost breakdown by model | Group costs per model |
| Create budget policy | Set monthly limit for agent/project/workspace |
| Budget enforcement | Agent exceeds budget → paused |
| Budget alert in inbox | Budget threshold → inbox notification |
| Costs page UI | Overview, breakdown tables, budget cards |

### 11. Channels & Communication (`office/channels.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Setup channel (Telegram/Slack) | Create channel for agent → stored |
| List channels | Shows configured channels |
| Delete channel | Channel removed |
| Inbound message relay | POST to inbound → task comment created → agent woken |

### 12. Inbox & Notifications (`office/inbox.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Empty inbox | No pending items |
| Approval inbox item | Create approval → shows in inbox |
| Decide approval (approve) | Click approve → approval resolved, task advances |
| Decide approval (reject) | Click reject → task returned to assignee |
| Budget alert inbox item | Budget exceeded → inbox alert |
| Agent error inbox item | Agent failure → inbox error |
| Permission request inbox | Agent requests tool permission → inbox notification |
| Respond to permission request | User approves/denies → agent continues/stops |

### 13. Agent Permissions & Clarifications (`office/agent-permissions.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Mock agent sends permission_request | Agent requests `bash` tool → notification appears |
| User approves permission | Approve → agent continues execution |
| User denies permission | Deny → agent receives denial, stops tool use |
| Auto-approve setting | Configure auto-approve → permissions auto-granted |
| Agent asks clarification question | Agent asks question → user sees it in chat/inbox |
| User answers clarification | Reply → agent receives answer, continues |
| Configurable approval agent | Setting: "reviewer agent handles permissions" → agent auto-responds |

### 14. Scheduler & Wakeup (`office/scheduler.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Task assigned → wakeup queued | Create task with assignee → wakeup in queue |
| Scheduler claims wakeup | Tick fires → wakeup claimed → agent session started |
| Wakeup with mock agent | Mock agent receives prompt with task context |
| Agent completes → task state updates | Mock agent finishes → task moves to DONE |
| Retry on failure | Agent errors → wakeup retried with backoff |
| Blocker resolution wakeup | Blocked task unblocked → assignee woken |
| Children completed wakeup | All subtasks done → parent agent woken |
| Idle timeout | Agent idle too long → session cleaned up |
| Wakeup coalescing | Multiple events → single wakeup (5s window) |

### 15. Dashboard & Activity (`office/dashboard.spec.ts`)

| Test | What it validates |
|------|-------------------|
| Dashboard metrics | Agent count, task count, spend, pending approvals |
| Activity feed | Actions logged (agent created, task created, etc.) |
| Activity filtering | Filter by type/action |
| Workspace settings | Update name/description |
| Org chart page | Shows agent hierarchy |
| Projects page | List, create, detail with task counts |

## Gaps Identified (Need Implementation Before Testing)

| Gap | Type | Impact |
|-----|------|--------|
| Documents lack HTTP routes | Backend | Can't test documents via REST API — only WebSocket |
| Attachments lack upload endpoint | Backend | Can't test file upload |
| Permission request → inbox notification | Backend | No inbox item for tool permissions (only approvals, budget, errors) |
| Configurable permission handling (agent vs human) | Backend + frontend | No setting to toggle auto-approve or delegate to agent |
| Labels not in issue list response | Backend | Issue list returns old JSON column, not junction table |
| No UI for setting execution policy on task | Frontend | Can only set via API, no dialog |
| No task documents UI component | Frontend | Documents section not wired in task detail |

## Implementation Priority Before Testing

1. **Add HTTP routes for documents** (CRUD + upload/download)
2. **Add permission request → inbox notification**
3. **Add configurable permission handling setting**
4. **Fix labels in issue response** (use junction table)
5. **Add task documents UI** (collapsible cards in task detail)
6. **Add execution policy UI** (stage selector in task create dialog)

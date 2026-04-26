# Orchestrate UI Reference

**Date:** 2026-04-26
**Purpose:** Detailed UI component specifications for all orchestrate pages. Referenced by wave plans. Based on Paperclip UI analysis, adapted for kandev's stack (`@kandev/ui` shadcn components, Tailwind CSS, lucide-react icons).

## Global Patterns

### Component imports
```tsx
import { Button } from '@kandev/ui/button';
import { Card, CardHeader, CardTitle, CardContent, CardFooter } from '@kandev/ui/card';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@kandev/ui/dialog';
import { Badge } from '@kandev/ui/badge';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Input } from '@kandev/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@kandev/ui/tabs';
import { Popover, PopoverContent, PopoverTrigger } from '@kandev/ui/popover';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@kandev/ui/collapsible';
import { ScrollArea } from '@kandev/ui/scroll-area';
import { Separator } from '@kandev/ui/separator';
import { Checkbox } from '@kandev/ui/checkbox';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
```

### Status colors (issue status)
```
backlog:     text-muted-foreground (gray circle outline)
todo:        text-blue-600 dark:text-blue-400 (blue circle outline)
in_progress: text-yellow-600 dark:text-yellow-400 (yellow circle outline)
in_review:   text-violet-600 dark:text-violet-400 (violet circle outline)
done:        text-green-600 dark:text-green-400 (green filled circle)
cancelled:   text-neutral-500 (gray strikethrough circle)
blocked:     text-red-600 dark:text-red-400 (red circle with corner dot)
```

### Status badges (pill variants)
```tsx
// Use @kandev/ui/badge with custom className
<Badge className="bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300">Todo</Badge>
<Badge className="bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300">In Progress</Badge>
<Badge className="bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300">Done</Badge>
<Badge className="bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300">Error</Badge>
```

### Agent status dots
```
idle:    bg-neutral-400 (gray)
working: bg-cyan-400 animate-pulse (animated cyan)
paused:  bg-yellow-400 (yellow)
stopped: bg-neutral-400 (gray, dimmed)
error:   bg-red-400 (red)
```

### Priority icons (from lucide-react)
```
critical: AlertTriangle, text-red-600
high:     ArrowUp, text-orange-600
medium:   Minus, text-yellow-600
low:      ArrowDown, text-blue-600
```

### Section headers (sidebar, page sections)
```tsx
<div className="px-3 py-1.5 text-[10px] font-medium uppercase tracking-widest font-mono text-muted-foreground/60">
  {label}
</div>
```

### Page title pattern
```tsx
<h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
  {pageTitle}
</h1>
```

---

## Sidebar (Wave 1)

### Layout
```
<aside className="w-60 h-full min-h-0 border-r border-border bg-background flex flex-col">
  {/* Top: workspace switcher + search, h-12 */}
  <div className="flex items-center gap-1 px-3 h-12 border-b border-border">
    {/* Reuse existing components/task/workspace-switcher.tsx -- same setActiveWorkspace + router.push flow */}
    {/* Orchestrate routes use /orchestrate?workspaceId=xxx to persist active workspace in URL */}
    <WorkspaceSwitcher />
    <Button variant="ghost" size="icon-sm"><Search className="h-4 w-4" /></Button>
  </div>

  {/* Nav: scrollable */}
  <nav className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 px-3 py-2">
    {/* Top actions */}
    <div className="flex flex-col gap-0.5">
      <SidebarNavItem icon={SquarePen} label="New Issue" onClick={openNewIssueDialog} />
      <SidebarNavItem icon={LayoutDashboard} label="Dashboard" to="/orchestrate" liveCount={liveRunCount} />
      <SidebarNavItem icon={Inbox} label="Inbox" to="/orchestrate/inbox" badge={inboxCount} />
    </div>

    {/* Work section */}
    <SidebarSection label="Work">
      <SidebarNavItem icon={CircleDot} label="Issues" to="/orchestrate/issues" />
      <SidebarNavItem icon={Repeat} label="Routines" to="/orchestrate/routines" />
    </SidebarSection>

    {/* Projects section (collapsible) */}
    <SidebarProjectsSection projects={projects} />

    {/* Agents section (collapsible) */}
    <SidebarAgentsSection agents={agents} />

    {/* Company section */}
    <SidebarSection label="Company">
      <SidebarNavItem icon={Network} label="Org" to="/orchestrate/company/org" />
      <SidebarNavItem icon={Boxes} label="Skills" to="/orchestrate/company/skills" />
      <SidebarNavItem icon={DollarSign} label="Costs" to="/orchestrate/company/costs" />
      <SidebarNavItem icon={History} label="Activity" to="/orchestrate/company/activity" />
      <SidebarNavItem icon={Settings} label="Settings" to="/orchestrate/company/settings" />
    </SidebarSection>
  </nav>
</aside>
```

### SidebarNavItem
```tsx
// Layout: flex items-center gap-2.5 px-3 py-2 text-[13px] font-medium rounded-md
// Active: bg-accent text-foreground
// Inactive: text-foreground/80 hover:bg-accent/50

// Badge (number): rounded-full px-1.5 py-0.5 text-xs bg-primary text-primary-foreground
// Badge (danger): bg-red-600/90 text-red-50
// Live count: animated pulsing blue dot + count text-[11px] text-blue-600
```

### SidebarAgentsSection
```tsx
// Collapsible with ChevronRight (rotates on open) + "Agents" header + Plus button
// Each agent item:
//   AgentIcon (lucide icon from agent config) + Name (truncated) + Status dot
//   If channel configured: platform icon (e.g. MessageCircle for Telegram)
//   If budget paused: PauseCircle icon in yellow
//   If live runs: pulsing blue dot + count
```

### SidebarProjectsSection
```tsx
// Collapsible with ChevronRight + "Projects" header + Plus button
// Each project item:
//   Color dot (h-3.5 w-3.5 rounded-sm) + Name (truncated)
//   Task count badge (if > 0)
```

---

## Dashboard (Wave 5E)

### Layout
```
<div className="space-y-6 p-6">
  {/* Metric cards: 4-column grid */}
  <div className="grid grid-cols-2 xl:grid-cols-4 gap-2">
    <MetricCard icon={Bot} value={agentCount} label="Agents Enabled" description={`${running} running, ${paused} paused, ${errors} errors`} />
    <MetricCard icon={CircleDot} value={tasksInProgress} label="Tasks In Progress" description={`${open} open, ${blocked} blocked`} />
    <MetricCard icon={DollarSign} value={monthSpend} label="Month Spend" description={`${pct}% of ${budget}`} />
    <MetricCard icon={ShieldCheck} value={pendingApprovals} label="Pending Approvals" />
  </div>

  {/* Activity charts: 4-column grid */}
  <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
    <ChartCard title="Run Activity" subtitle="Last 14 days"><RunActivityChart data={runData} /></ChartCard>
    <ChartCard title="By Priority"><PriorityChart data={priorityData} /></ChartCard>
    <ChartCard title="By Status"><StatusChart data={statusData} /></ChartCard>
    <ChartCard title="Success Rate"><SuccessRateChart rate={successRate} /></ChartCard>
  </div>

  {/* Recent activity + recent tasks: 2-column grid */}
  <div className="grid md:grid-cols-2 gap-4">
    <RecentActivitySection entries={recentActivity} />
    <RecentTasksSection tasks={recentTasks} />
  </div>
</div>
```

### MetricCard
```tsx
// Card with: icon (top-right, text-muted-foreground), value (text-2xl sm:text-3xl font-bold), label (text-xs text-muted-foreground), description (text-xs)
<Card className="p-4 cursor-pointer hover:bg-accent/50 transition-colors">
  <div className="flex justify-between items-start">
    <div>
      <p className="text-2xl sm:text-3xl font-bold">{value}</p>
      <p className="text-xs sm:text-sm text-muted-foreground mt-1">{label}</p>
      <p className="text-xs text-muted-foreground">{description}</p>
    </div>
    <Icon className="h-5 w-5 text-muted-foreground" />
  </div>
</Card>
```

### Charts (custom, no library)
```tsx
// Stacked bar chart: 14-day window, each day is a flex column
// Bar colors: emerald-500 (success), red-500 (failed), neutral-500 (other)
// Height: 80px total, bars proportional
// Date labels: show day 0, 6, 13 only (text-[10px] text-muted-foreground)
// Legend: colored dots + labels below chart
```

---

## Issues List (Wave 3B)

### Layout
```
<div className="space-y-4 p-6">
  {/* Toolbar */}
  <div className="flex items-center gap-2">
    <Button onClick={openNewIssue}><Plus className="h-4 w-4 mr-1" /> New Issue</Button>
    <div className="relative flex-1 max-w-[300px]">
      <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
      <Input placeholder="Search issues..." className="pl-8 h-9 text-sm" />
    </div>
    <div className="ml-auto flex items-center gap-1">
      <Button variant={viewMode === 'list' ? 'secondary' : 'ghost'} size="icon-sm"><List /></Button>
      <Button variant={viewMode === 'board' ? 'secondary' : 'ghost'} size="icon-sm"><Columns3 /></Button>
      <Separator orientation="vertical" className="h-6 mx-1" />
      <Button variant={nestingEnabled ? 'secondary' : 'ghost'} size="icon-sm"><ListTree /></Button>
      <ColumnPickerPopover />
      <FilterPopover />
      <SortPopover />
      <GroupPopover />
    </div>
  </div>

  {/* Issue list */}
  <div className="border border-border rounded-lg divide-y divide-border">
    {issues.map(issue => <IssueRow key={issue.id} issue={issue} />)}
  </div>
</div>
```

### IssueRow
```tsx
// Layout: flex items-center gap-2 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors cursor-pointer
// Nesting: ml-{level * 6} for indented children, collapse/expand chevron on parents
<div className="flex items-center gap-2 px-4 py-2.5 text-sm hover:bg-accent/50 transition-colors cursor-pointer">
  {hasChildren && <ChevronRight className={cn("h-3.5 w-3.5 transition-transform", expanded && "rotate-90")} />}
  <StatusIcon status={issue.status} className="h-4 w-4 shrink-0" />
  <span className="text-xs text-muted-foreground font-mono shrink-0">{issue.identifier}</span>
  <span className="flex-1 truncate">{issue.title}</span>
  {issue.assignee && <AgentAvatar agent={issue.assignee} size="xs" />}
  <span className="text-xs text-muted-foreground shrink-0">{timeAgo(issue.updatedAt)}</span>
</div>
```

---

## Task Detail - Simple Mode (Wave 3C)

### Layout
```
<div className="flex h-full">
  {/* Main content */}
  <div className="flex-1 min-w-0 overflow-y-auto p-6">
    {/* Breadcrumb */}
    <Breadcrumb>
      <BreadcrumbList>
        <BreadcrumbItem><BreadcrumbLink href="/orchestrate/issues">Issues</BreadcrumbLink></BreadcrumbItem>
        {parentTask && <><BreadcrumbSeparator /><BreadcrumbItem><BreadcrumbLink>{parentTask.title}</BreadcrumbLink></BreadcrumbItem></>}
        <BreadcrumbSeparator />
        <BreadcrumbItem><BreadcrumbPage>{task.title}</BreadcrumbPage></BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>

    {/* Task header */}
    <div className="flex items-center gap-2 mt-4">
      <StatusIcon status={task.status} />
      <span className="text-sm font-mono text-muted-foreground">{task.identifier}</span>
      <Badge variant="outline">{task.project?.name}</Badge>
      <div className="ml-auto flex gap-1">
        <Button variant="ghost" size="icon-sm" onClick={toggleAdvancedMode}><Code2 /></Button>
        <Button variant="ghost" size="icon-sm"><Copy /></Button>
        <DropdownMenu>...</DropdownMenu>
      </div>
    </div>

    {/* Title + Description */}
    <h1 className="text-xl font-semibold mt-4">{task.title}</h1>
    <div className="prose prose-sm mt-4 max-w-none"><Markdown>{task.description}</Markdown></div>

    {/* Action buttons */}
    <div className="flex gap-2 mt-6">
      <Button variant="outline" size="sm"><Plus className="h-3.5 w-3.5 mr-1" /> New Sub-Issue</Button>
      <Button variant="outline" size="sm"><Paperclip className="h-3.5 w-3.5 mr-1" /> Upload attachment</Button>
      <Button variant="outline" size="sm"><Plus className="h-3.5 w-3.5 mr-1" /> New document</Button>
    </div>

    {/* Chat / Activity tabs */}
    <Tabs defaultValue="chat" className="mt-6">
      <TabsList><TabsTrigger value="chat">Chat</TabsTrigger><TabsTrigger value="activity">Activity</TabsTrigger></TabsList>
      <TabsContent value="chat"><ChatThread taskId={task.id} /></TabsContent>
      <TabsContent value="activity"><ActivityTimeline taskId={task.id} /></TabsContent>
    </Tabs>

    {/* Sub-issues section */}
    {task.children.length > 0 && (
      <div className="mt-8">
        <h2 className="text-sm font-semibold mb-4">Sub-issues</h2>
        <SubIssuesList parentId={task.id} />
      </div>
    )}
  </div>

  {/* Properties panel (right sidebar, ~320px) */}
  <div className="w-80 border-l border-border shrink-0 overflow-y-auto p-4">
    <TaskProperties task={task} onUpdate={updateTask} />
  </div>
</div>
```

### TaskProperties (right panel)
```tsx
// Each property: flex items-center justify-between py-2 border-b border-border/50
// Label: text-sm text-muted-foreground w-24 shrink-0
// Value: editable popover/select/input
<div className="space-y-0">
  <PropertyRow label="Status"><StatusSelect value={task.status} onChange={...} /></PropertyRow>
  <PropertyRow label="Priority"><PrioritySelect value={task.priority} onChange={...} /></PropertyRow>
  <PropertyRow label="Labels"><LabelMultiSelect values={task.labels} onChange={...} /></PropertyRow>
  <PropertyRow label="Assignee"><AgentPicker value={task.assigneeAgentInstanceId} onChange={...} /></PropertyRow>
  <PropertyRow label="Project"><ProjectPicker value={task.projectId} onChange={...} /></PropertyRow>
  <PropertyRow label="Parent">{task.parent ? <Link>{task.parent.identifier} {task.parent.title}</Link> : "None"}</PropertyRow>
  <Separator className="my-2" />
  <PropertyRow label="Blocked by"><BlockerMultiSelect values={task.blockedBy} onChange={...} /></PropertyRow>
  <PropertyRow label="Blocking"><BlockingList items={task.blocking} /></PropertyRow>
  <PropertyRow label="Sub-issues"><SubIssueList items={task.children} onAdd={...} /></PropertyRow>
  <Separator className="my-2" />
  <PropertyRow label="Reviewers"><ParticipantMultiSelect values={task.reviewers} onChange={...} /></PropertyRow>
  <PropertyRow label="Approvers"><ParticipantMultiSelect values={task.approvers} onChange={...} /></PropertyRow>
  <Separator className="my-2" />
  <PropertyRow label="Created by"><Identity name={task.createdBy} /></PropertyRow>
  <PropertyRow label="Started">{task.startedAt ? formatDate(task.startedAt) : "—"}</PropertyRow>
  <PropertyRow label="Completed">{task.completedAt ? formatDate(task.completedAt) : "—"}</PropertyRow>
  <PropertyRow label="Created">{formatDate(task.createdAt)}</PropertyRow>
  <PropertyRow label="Updated">{timeAgo(task.updatedAt)}</PropertyRow>
</div>
```

### ChatThread
```tsx
// Agent run entries: collapsible tool call details
// Each agent message block:
<div className="flex gap-3 py-3">
  <AgentAvatar agent={message.agent} size="sm" />
  <div className="flex-1 min-w-0">
    <div className="flex items-center gap-2">
      <span className="font-medium text-sm">{message.agent.name}</span>
      <span className="text-xs text-muted-foreground">{message.status} after {duration}</span>
      <span className="text-xs text-muted-foreground">{timeAgo(message.createdAt)}</span>
    </div>
    <div className="prose prose-sm mt-1">{message.content}</div>
    {message.toolCalls && (
      <Collapsible>
        <CollapsibleTrigger className="flex items-center gap-1 text-xs text-muted-foreground mt-1">
          <Code2 className="h-3 w-3" /> Worked · ran {message.toolCalls.length} commands
          <ChevronDown className="h-3 w-3" />
        </CollapsibleTrigger>
        <CollapsibleContent>...</CollapsibleContent>
      </Collapsible>
    )}
  </div>
</div>
```

---

## Task Detail - Advanced Mode (Wave 3C)

When the user clicks the Code2 toggle button on simple mode, the layout switches to a full kandev dockview experience, but within the orchestrate chrome (sidebar + topbar remain).

### Layout
```
+------------------+------------------------------------------------------------+
| Orchestrate      | Orchestrate Topbar                                         |
| Sidebar          |  [Simple mode toggle] [Task: KAN-3 title] [Agent status]   |
| (unchanged,      |------------------------------------------------------------ |
|  stays visible)  | Dockview                                    | Right Panel  |
|                  |                                             |              |
|                  | +----------+ +----------+ +----------+     | Files        |
|                  | | Chat tab | |Terminal  | | Plan tab |     | Changes      |
|                  | +----------+ +----------+ +----------+     | Git status   |
|                  |                                             |              |
|                  | [Active tab content fills this area]        |              |
|                  |                                             |              |
|                  | [Message input bar at bottom of chat]       |              |
|                  |                                             |              |
+------------------+---------------------------------------------+--------------+
```

### Implementation
```tsx
// The page component toggles between simple and advanced based on URL query param
// /orchestrate/issues/[id] -> simple mode
// /orchestrate/issues/[id]?mode=advanced -> advanced mode

function OrchestrateTaskDetail({ task }: Props) {
  const [mode, setMode] = useQueryParam('mode', 'simple');

  if (mode === 'advanced') {
    return <OrchestrateAdvancedMode task={task} onToggleSimple={() => setMode('simple')} />;
  }
  return <OrchestrateSimpleMode task={task} onToggleAdvanced={() => setMode('advanced')} />;
}
```

### Advanced mode component
```tsx
function OrchestrateAdvancedMode({ task, onToggleSimple }: Props) {
  // Auto-launch ACP session on mount (no prompt, no tokens consumed)
  useEffect(() => {
    startOrResumeSession(task.id, task.assigneeAgentInstanceId);
  }, [task.id]);

  return (
    <div className="flex flex-col h-full">
      {/* Topbar within dockview area */}
      <div className="flex items-center gap-2 px-4 h-10 border-b border-border bg-background shrink-0">
        <Button variant="ghost" size="icon-sm" onClick={onToggleSimple}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon-sm" onClick={onToggleSimple}>
              <LayoutList className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Switch to simple mode</TooltipContent>
        </Tooltip>
        <Separator orientation="vertical" className="h-5" />
        <StatusIcon status={task.status} className="h-4 w-4" />
        <span className="text-xs font-mono text-muted-foreground">{task.identifier}</span>
        <span className="text-sm font-medium truncate">{task.title}</span>
        <div className="ml-auto flex items-center gap-2">
          {sessionActive && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <div className="h-2 w-2 rounded-full bg-cyan-400 animate-pulse" />
              Agent session active
            </div>
          )}
          <AgentAvatar agent={task.assignee} size="xs" />
        </div>
      </div>

      {/* Dockview area: reuse existing kandev dockview components */}
      <div className="flex-1 min-h-0">
        <DockviewReact
          className="h-full"
          components={{
            chat: ChatPanel,        // existing kandev component
            terminal: TerminalPanel, // existing kandev component
            plan: PlanPanel,         // existing kandev component
          }}
          rightComponents={{
            files: FilesPanel,       // existing kandev component
            changes: ChangesPanel,   // existing kandev component
          }}
          // Fixed layout: no drag to rearrange, no presets, no save/load
          // Left panel tabs: Chat (default active), Terminal, Plan
          // Right panel: Files, Changes (collapsible)
        />
      </div>
    </div>
  );
}
```

### Key differences from existing kandev task detail
| Aspect | Existing `/t/[taskId]` | Orchestrate advanced mode |
|--------|----------------------|--------------------------|
| Sidebar | Kandev sidebar with task list, workflows | Orchestrate sidebar (agents, projects, etc.) |
| Topbar | Kandev topbar with workspace selector | Orchestrate topbar with back-to-simple toggle |
| Layout presets | User can save/load, rearrange panels | Fixed layout, no presets |
| Left panel | Configurable (task list, repos, settings) | Not shown (sidebar is orchestrate nav) |
| Session start | User clicks "Start" or sends message | Auto-started/resumed on entering advanced mode |
| Session lifecycle | Long-running, user-initiated | Can be one-shot (scheduler) or interactive (advanced mode) |

### Session management in advanced mode
```tsx
// On entering advanced mode:
// 1. Check if there's an existing session for this task + agent instance
// 2. If yes: resume it (ACP session/load with existing sessionId)
// 3. If no: create a new session (ACP session/new, no initial prompt)
// 4. Session is idle until user types in chat input
// 5. On leaving advanced mode: session stays open (not stopped)
// 6. Next scheduler wakeup can resume the same session if context matches

async function startOrResumeSession(taskId: string, agentInstanceId: string) {
  const existingSession = await getActiveSession(taskId, agentInstanceId);
  if (existingSession) {
    await resumeSession(existingSession.id); // ACP session/load
  } else {
    await createSession(taskId, agentInstanceId); // ACP session/new, no prompt
  }
  // Session is now ready -- agent idle, no tokens consumed
  // When user types in ChatPanel, it sends a prompt via existing WS
}
```

### Toggle button appearance
```tsx
// In simple mode: Code2 icon in header to switch to advanced
<Tooltip>
  <TooltipTrigger asChild>
    <Button variant="ghost" size="icon-sm" onClick={toggleAdvancedMode}>
      <Code2 className="h-4 w-4" />
    </Button>
  </TooltipTrigger>
  <TooltipContent>Advanced mode (terminal, files, plan)</TooltipContent>
</Tooltip>

// In advanced mode: LayoutList icon + ArrowLeft to switch back to simple
<Tooltip>
  <TooltipTrigger asChild>
    <Button variant="ghost" size="icon-sm" onClick={onToggleSimple}>
      <LayoutList className="h-4 w-4" />
    </Button>
  </TooltipTrigger>
  <TooltipContent>Switch to simple mode</TooltipContent>
</Tooltip>
```

---

## New Issue Dialog (Wave 3D)

```tsx
<Dialog>
  <DialogContent className="max-w-2xl">
    {/* Header */}
    <DialogHeader>
      <div className="flex items-center gap-2">
        <Badge variant="outline" className="font-mono text-xs">KAN</Badge>
        <span className="text-sm text-muted-foreground">New issue</span>
      </div>
    </DialogHeader>

    {/* Title */}
    <Textarea placeholder="Issue title" className="text-lg font-medium border-0 resize-none p-0 focus-visible:ring-0 min-h-[40px]" />

    {/* Quick selector row */}
    <div className="flex items-center gap-2 text-sm text-muted-foreground">
      <span>For</span>
      <Button variant="outline" size="sm">{assignee || "Assignee"}</Button>
      <span>in</span>
      <Button variant="outline" size="sm">{project || "Project"}</Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm"><MoreHorizontal /></Button></DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem><Eye className="h-4 w-4 mr-2" /> Reviewer</DropdownMenuItem>
          <DropdownMenuItem><CheckCircle2 className="h-4 w-4 mr-2" /> Approver</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>

    {/* Description */}
    <Textarea placeholder="Add description..." className="min-h-[120px] text-sm" />

    {/* Bottom bar */}
    <div className="flex items-center gap-2 pt-2 border-t border-border">
      <Button variant="outline" size="sm"><CircleDot className="h-3.5 w-3.5 mr-1" /> Todo</Button>
      <Button variant="outline" size="sm"><Minus className="h-3.5 w-3.5 mr-1" /> Priority</Button>
      <Button variant="outline" size="sm"><Paperclip className="h-3.5 w-3.5 mr-1" /> Upload</Button>
      <Button variant="ghost" size="icon-sm"><MoreHorizontal /></Button>
    </div>

    {/* Footer */}
    <div className="flex justify-between pt-4">
      <Button variant="ghost" className="text-muted-foreground">Discard Draft</Button>
      <Button>Create Issue</Button>
    </div>
  </DialogContent>
</Dialog>
```

---

## Agent Detail Page (Wave 2A)

### Tabs: Overview | Skills | Runs | Memory | Channels
```tsx
<Tabs defaultValue="overview">
  <TabsList>
    <TabsTrigger value="overview">Overview</TabsTrigger>
    <TabsTrigger value="skills">Skills</TabsTrigger>
    <TabsTrigger value="runs">Runs</TabsTrigger>
    <TabsTrigger value="memory">Memory</TabsTrigger>
    <TabsTrigger value="channels">Channels</TabsTrigger>
  </TabsList>
</Tabs>
```

### Overview tab
```tsx
<div className="space-y-6">
  {/* Identity card */}
  <Card>
    <CardContent className="flex items-center gap-4 pt-6">
      <AgentIcon icon={agent.icon} className="h-12 w-12" />
      <div>
        <h2 className="text-lg font-semibold">{agent.name}</h2>
        <div className="flex items-center gap-2 mt-1">
          <Badge variant="outline">{agent.role}</Badge>
          <StatusDot status={agent.status} />
          <span className="text-sm text-muted-foreground">{agent.status}</span>
        </div>
      </div>
      <div className="ml-auto"><BudgetGauge agent={agent} /></div>
    </CardContent>
  </Card>

  {/* Activity charts: same 4-chart grid as dashboard */}
  <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
    <ChartCard title="Run Activity" subtitle="Last 14 days">...</ChartCard>
    ...
  </div>
</div>
```

---

## Costs Page (Wave 5A)

### Layout
```tsx
<Tabs defaultValue="overview">
  <TabsList>
    <TabsTrigger value="overview">Overview</TabsTrigger>
    <TabsTrigger value="budgets">Budgets</TabsTrigger>
  </TabsList>

  <TabsContent value="overview">
    {/* Date range picker */}
    <div className="flex gap-2 mb-4">
      <Button variant={range === 'mtd' ? 'secondary' : 'outline'} size="sm">MTD</Button>
      <Button variant={range === '30d' ? 'secondary' : 'outline'} size="sm">Last 30 days</Button>
    </div>

    {/* Summary metrics */}
    <div className="grid grid-cols-2 xl:grid-cols-4 gap-2 mb-6">
      <MetricCard icon={DollarSign} value={totalSpend} label="Total Spend" />
      ...
    </div>

    {/* Breakdown tables */}
    <div className="space-y-6">
      <CostByAgentTable data={agentCosts} />
      <CostByProjectTable data={projectCosts} />
      <CostByModelTable data={modelCosts} />
    </div>
  </TabsContent>

  <TabsContent value="budgets">
    {/* Budget policy cards */}
    <div className="grid gap-4 md:grid-cols-2">
      {policies.map(p => <BudgetPolicyCard key={p.id} policy={p} />)}
    </div>
  </TabsContent>
</Tabs>
```

### BudgetPolicyCard
```tsx
<Card>
  <CardHeader>
    <CardTitle className="text-sm">{scope.name}</CardTitle>
    <StatusBadge status={budgetStatus} /> {/* Healthy / Warning / Paused */}
  </CardHeader>
  <CardContent>
    <div className="space-y-2">
      <div className="flex justify-between text-sm">
        <span>Observed</span>
        <span>${observed} ({pct}%)</span>
      </div>
      {/* Progress bar */}
      <div className="h-2 bg-muted rounded-full overflow-hidden">
        <div className={cn("h-full rounded-full", pct > 90 ? "bg-red-500" : pct > 70 ? "bg-yellow-500" : "bg-green-500")} style={{width: `${pct}%`}} />
      </div>
      <div className="flex justify-between text-xs text-muted-foreground">
        <span>Budget: ${limit}</span>
        <span>Remaining: ${remaining}</span>
      </div>
    </div>
  </CardContent>
</Card>
```

---

## Inbox Page (Wave 5D)

### Layout
```tsx
<div className="space-y-4 p-6">
  {/* Tabs: Mine | Recent | Unread | All */}
  <Tabs defaultValue="mine">
    <div className="flex items-center justify-between">
      <TabsList>
        <TabsTrigger value="mine">Mine</TabsTrigger>
        <TabsTrigger value="recent">Recent</TabsTrigger>
        <TabsTrigger value="unread">Unread</TabsTrigger>
        <TabsTrigger value="all">All</TabsTrigger>
      </TabsList>
      <Input placeholder="Search..." className="w-[220px] h-8 pl-8 text-xs" />
    </div>
  </Tabs>

  {/* Items list */}
  <div className="border border-border rounded-lg divide-y divide-border">
    {items.map(item => {
      switch (item.type) {
        case 'approval': return <ApprovalRow item={item} />;
        case 'budget_alert': return <BudgetAlertRow item={item} />;
        case 'agent_error': return <AgentErrorRow item={item} />;
        case 'task_review': return <TaskReviewRow item={item} />;
      }
    })}
  </div>
</div>
```

### ApprovalRow
```tsx
<div className="flex items-center gap-3 px-4 py-3 hover:bg-accent/50 transition-colors">
  <div className="h-8 w-8 rounded-full bg-muted flex items-center justify-center shrink-0">
    <ShieldCheck className="h-4 w-4 text-muted-foreground" />
  </div>
  <div className="flex-1 min-w-0">
    <p className="text-sm font-medium truncate">{approvalLabel}</p>
    <p className="text-xs text-muted-foreground">requested by {requester} · {timeAgo}</p>
  </div>
  <StatusBadge status={item.status} />
  {item.status === 'pending' && (
    <div className="flex gap-2 shrink-0">
      <Button size="sm" className="bg-green-700 text-white hover:bg-green-800">Approve</Button>
      <Button size="sm" variant="destructive">Reject</Button>
    </div>
  )}
</div>
```

---

## Routines Page (Wave 6A)

### Layout
```tsx
<div className="space-y-4 p-6">
  <Tabs defaultValue="routines">
    <TabsList>
      <TabsTrigger value="routines">Routines</TabsTrigger>
      <TabsTrigger value="runs">Runs</TabsTrigger>
    </TabsList>
  </Tabs>

  {/* Routine list */}
  <div className="border border-border rounded-lg divide-y divide-border">
    {routines.map(r => <RoutineRow key={r.id} routine={r} />)}
  </div>
</div>
```

### RoutineRow
```tsx
<div className="flex items-center gap-3 px-4 py-3 hover:bg-accent/50 transition-colors cursor-pointer">
  <div className="flex-1 min-w-0">
    <p className="text-sm font-medium truncate">{routine.name}</p>
    <div className="flex items-center gap-2 mt-0.5 text-xs text-muted-foreground">
      {routine.project && <><span className="h-2.5 w-2.5 rounded-sm" style={{backgroundColor: routine.project.color}} /> {routine.project.name}</>}
      {routine.assignee && <><AgentIcon size="xs" /> {routine.assignee.name}</>}
      <span>{routine.lastRunAt ? `${timeAgo(routine.lastRunAt)} · ${routine.lastRunStatus}` : 'Never run'}</span>
    </div>
  </div>
  <Badge variant={routine.status === 'active' ? 'default' : 'secondary'}>{routine.status === 'active' ? 'On' : 'Off'}</Badge>
  <Switch checked={routine.status === 'active'} onCheckedChange={toggleRoutine} />
  <DropdownMenu>{/* Edit, Run now, Pause, Archive */}</DropdownMenu>
</div>
```

---

## Org Chart (Wave 6B)

### Layout
```tsx
// Custom tree layout (no library dependency)
// Constants: CARD_W=200, CARD_H=100, GAP_X=32, GAP_Y=80

// Zoom controls: top-right
<div className="absolute top-4 right-4 flex flex-col gap-1">
  <Button variant="outline" size="icon-sm" onClick={zoomIn}><Plus /></Button>
  <Button variant="outline" size="icon-sm" onClick={zoomOut}><Minus /></Button>
  <Button variant="outline" size="sm" onClick={fitToView}>Fit</Button>
</div>

// Each node card:
<div className="absolute border border-border rounded-lg bg-card p-3 w-[200px] cursor-pointer hover:border-primary transition-colors" style={{left: node.x, top: node.y}}>
  <div className="flex items-start gap-2">
    <AgentIcon icon={node.icon} className="h-8 w-8 shrink-0" />
    <div className="min-w-0">
      <p className="text-sm font-medium truncate">{node.name}</p>
      <p className="text-xs text-muted-foreground">{node.role}</p>
      <p className="text-xs text-muted-foreground">{node.adapterType}</p>
    </div>
  </div>
  <StatusDot status={node.status} className="absolute bottom-2 left-2" />
</div>

// Connection lines: SVG <line> elements between parent bottom-center and child top-center
```

---

## Skills Page (Wave 2B)

### Layout
```tsx
// Two-column split: skill list (left, ~300px) | skill editor (right, flex-1)
<div className="flex h-full">
  <div className="w-[300px] border-r border-border overflow-y-auto p-4">
    <Button className="w-full mb-4"><Plus className="h-4 w-4 mr-1" /> Add Skill</Button>
    <div className="space-y-1">
      {skills.map(s => (
        <div key={s.id} className={cn("flex items-center gap-2 px-3 py-2 rounded-md text-sm cursor-pointer", selected === s.id ? "bg-accent" : "hover:bg-accent/50")}>
          <Boxes className="h-4 w-4 text-muted-foreground shrink-0" />
          <span className="truncate">{s.name}</span>
          <Badge variant="outline" className="ml-auto text-[10px]">{s.sourceType}</Badge>
        </div>
      ))}
    </div>
  </div>
  <div className="flex-1 p-6 overflow-y-auto">
    {selectedSkill ? <SkillEditor skill={selectedSkill} /> : <EmptyState icon={Boxes} message="Select a skill to view" />}
  </div>
</div>
```

---

## New Agent Page (Wave 2A)

### Layout
```tsx
<div className="max-w-2xl mx-auto p-6 space-y-8">
  {/* Identity section */}
  <div className="space-y-4">
    <Input placeholder="Agent name" className="text-lg font-medium h-12" autoFocus />
    <Input placeholder="Title (e.g. VP Engineering)" className="text-sm" />
    <div className="flex flex-wrap gap-2">
      <RoleSelector value={role} onChange={setRole} />
      <ReportsToSelector value={reportsTo} onChange={setReportsTo} agents={agents} />
    </div>
  </div>

  {/* Configuration */}
  <div className="space-y-4">
    <h3 className="text-sm font-semibold">Configuration</h3>
    <AgentProfileSelector value={profileId} onChange={setProfileId} />
    <div className="flex gap-4">
      <div className="flex-1"><Label>Budget (monthly)</Label><Input type="number" prefix="$" /></div>
      <div className="flex-1"><Label>Max concurrent</Label><Input type="number" defaultValue={1} /></div>
    </div>
  </div>

  {/* Executor preference (optional override) */}
  <div className="space-y-4">
    <h3 className="text-sm font-semibold">Executor Preference</h3>
    <p className="text-xs text-muted-foreground">Override the project/workspace default executor for this agent. Leave empty to inherit.</p>
    <Select value={executorType} onValueChange={setExecutorType}>
      <SelectTrigger><SelectValue placeholder="Inherit from project/workspace" /></SelectTrigger>
      <SelectContent>
        <SelectItem value="">Inherit</SelectItem>
        <SelectItem value="local_pc">Local (standalone)</SelectItem>
        <SelectItem value="local_docker">Local Docker</SelectItem>
        <SelectItem value="sprites">Sprites (remote sandbox)</SelectItem>
        <SelectItem value="remote_docker">Remote Docker</SelectItem>
      </SelectContent>
    </Select>
    {executorType === 'local_docker' && (
      <>
        <Input placeholder="Docker image (e.g. node:20-slim)" value={image} onChange={...} />
        <div className="flex gap-4">
          <div className="flex-1"><Label>Memory (MB)</Label><Input type="number" placeholder="4096" /></div>
          <div className="flex-1"><Label>CPU cores</Label><Input type="number" placeholder="2" /></div>
        </div>
      </>
    )}
  </div>

  {/* Skills */}
  <div className="space-y-4">
    <h3 className="text-sm font-semibold">Skills</h3>
    <div className="space-y-2">
      {availableSkills.map(s => (
        <label key={s.id} className="flex items-center gap-3 py-1.5 cursor-pointer">
          <Checkbox checked={selectedSkills.includes(s.id)} onCheckedChange={...} />
          <span className="text-sm">{s.name}</span>
          <span className="text-xs text-muted-foreground ml-auto">{s.slug}</span>
        </label>
      ))}
    </div>
  </div>

  {/* Footer */}
  <div className="flex justify-end gap-2 pt-4 border-t border-border">
    <Button variant="ghost">Cancel</Button>
    <Button>Create Agent</Button>
  </div>
</div>
```

---

## Activity Page (Wave 5C)

### Layout
```tsx
<div className="space-y-4 p-6">
  <div className="flex justify-between items-center">
    <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">Activity</h1>
    <Select value={filterType} onValueChange={setFilterType}>
      <SelectTrigger className="w-[140px] h-8 text-xs">...</SelectTrigger>
      <SelectContent>{/* All types, agent, task, approval, etc. */}</SelectContent>
    </Select>
  </div>

  <div className="border border-border rounded-lg divide-y divide-border">
    {entries.map(e => <ActivityRow key={e.id} entry={e} />)}
  </div>
</div>
```

### ActivityRow
```tsx
<div className="flex items-start gap-3 px-4 py-2 text-sm hover:bg-accent/50 transition-colors cursor-pointer">
  <AgentAvatar name={entry.actorName} size="xs" />
  <div className="flex-1 min-w-0">
    <span className="font-medium">{entry.actorName}</span>
    <span className="text-muted-foreground"> {entry.actionVerb} </span>
    <Link className="font-medium hover:underline">{entry.targetName}</Link>
  </div>
  <span className="text-xs text-muted-foreground shrink-0">{timeAgo(entry.createdAt)}</span>
</div>
```

---

## Settings Page (Wave 1, expanded in Wave 7D)

### Layout
```tsx
<div className="max-w-3xl mx-auto p-6 space-y-8">
  {/* Workspace info */}
  <section className="space-y-4">
    <h2 className="text-sm font-semibold">Workspace</h2>
    <Input label="Name" value={workspace.name} />
    <Textarea label="Description" value={workspace.description} />
  </section>

  {/* Approval defaults */}
  <section className="space-y-4">
    <h2 className="text-sm font-semibold">Approval Settings</h2>
    <label className="flex items-center gap-3"><Checkbox /> Require approval for new agents</label>
    <label className="flex items-center gap-3"><Checkbox /> Require approval for task completion</label>
    <label className="flex items-center gap-3"><Checkbox /> Require approval for skill changes</label>
  </section>

  {/* Config sync (Wave 7D) */}
  <section className="space-y-4">
    <h2 className="text-sm font-semibold">Configuration</h2>
    <div className="flex gap-2">
      <Button variant="outline"><Download className="h-4 w-4 mr-1" /> Export</Button>
      <Button variant="outline"><Upload className="h-4 w-4 mr-1" /> Import</Button>
    </div>
  </section>
</div>
```

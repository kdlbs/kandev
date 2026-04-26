// --- Orchestrate entity types ---

export type AgentRole = "ceo" | "worker" | "specialist" | "assistant";
export type AgentStatus = "idle" | "working" | "paused" | "stopped" | "pending_approval";

export type AgentInstance = {
  id: string;
  workspaceId: string;
  name: string;
  agentProfileId?: string;
  role: AgentRole;
  icon?: string;
  status: AgentStatus;
  reportsTo?: string;
  permissions?: Record<string, unknown>;
  budgetMonthlyCents: number;
  maxConcurrentSessions: number;
  desiredSkills?: string[];
  executorPreference?: {
    type?: string;
    image?: string;
    resource_limits?: Record<string, unknown>;
    environment_id?: string;
  };
  pauseReason?: string;
  createdAt: string;
  updatedAt: string;
};

export type SkillSourceType = "inline" | "local_path" | "git";

export type Skill = {
  id: string;
  workspaceId: string;
  name: string;
  slug: string;
  description?: string;
  sourceType: SkillSourceType;
  sourceLocator?: string;
  content?: string;
  fileInventory?: string[];
  createdByAgentInstanceId?: string;
  createdAt: string;
  updatedAt: string;
};

export type ProjectStatus = "active" | "completed" | "on_hold" | "archived";

export type TaskCounts = {
  total: number;
  in_progress: number;
  done: number;
  blocked: number;
};

export type Project = {
  id: string;
  workspaceId: string;
  name: string;
  description?: string;
  status: ProjectStatus;
  leadAgentInstanceId?: string;
  color?: string;
  budgetCents?: number;
  repositories?: string[];
  executorConfig?: Record<string, unknown>;
  taskCounts?: TaskCounts;
  createdAt: string;
  updatedAt: string;
};

export type ApprovalType =
  | "hire_agent"
  | "budget_increase"
  | "board_approval"
  | "task_review"
  | "skill_creation";
export type ApprovalStatus = "pending" | "approved" | "rejected";

export type Approval = {
  id: string;
  workspaceId: string;
  type: ApprovalType;
  requestedByAgentInstanceId?: string;
  status: ApprovalStatus;
  payload?: Record<string, unknown>;
  decisionNote?: string;
  decidedBy?: string;
  decidedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type ActivityEntry = {
  id: string;
  workspaceId: string;
  actorType: "user" | "agent" | "system";
  actorId: string;
  action: string;
  targetType?: string;
  targetId?: string;
  details?: Record<string, unknown>;
  createdAt: string;
};

export type CostBreakdownItem = {
  group_key: string;
  total_cents: number;
  count: number;
};

export type CostSummary = {
  totalCents: number;
  byAgent: Array<{ agentInstanceId: string; name: string; costCents: number }>;
  byProject: Array<{ projectId: string; name: string; costCents: number }>;
  byModel: Array<{ model: string; costCents: number; tokensIn: number; tokensOut: number }>;
};

export type BudgetPolicy = {
  id: string;
  workspaceId: string;
  scopeType: "agent" | "project" | "workspace";
  scopeId: string;
  limitCents: number;
  period: "monthly" | "total";
  alertThresholdPct: number;
  actionOnExceed: "notify_only" | "pause_agent" | "block_new_tasks";
  createdAt: string;
  updatedAt: string;
};

export type RoutineStatus = "active" | "paused";

export type Routine = {
  id: string;
  workspaceId: string;
  name: string;
  description?: string;
  taskTemplate: Record<string, unknown>;
  assigneeAgentInstanceId?: string;
  status: RoutineStatus;
  concurrencyPolicy: string;
  variables?: Record<string, unknown>;
  lastRunAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type InboxItem = {
  id: string;
  type: "approval" | "budget_alert" | "agent_error" | "task_review";
  title: string;
  description?: string;
  status: string;
  createdAt: string;
  payload?: Record<string, unknown>;
};

export type WakeupEntry = {
  id: string;
  agentInstanceId: string;
  reason: string;
  status: "queued" | "claimed" | "finished" | "failed";
  requestedAt: string;
};

export type OrchestrateIssueStatus =
  | "backlog"
  | "todo"
  | "in_progress"
  | "in_review"
  | "blocked"
  | "done"
  | "cancelled";

export type OrchestrateIssuePriority = "critical" | "high" | "medium" | "low" | "none";

export type OrchestrateIssue = {
  id: string;
  workspaceId: string;
  identifier: string;
  title: string;
  description?: string;
  status: OrchestrateIssueStatus;
  priority: OrchestrateIssuePriority;
  parentId?: string;
  projectId?: string;
  assigneeAgentInstanceId?: string;
  labels?: string[];
  createdAt: string;
  updatedAt: string;
};

export type IssueFilterState = {
  statuses: OrchestrateIssueStatus[];
  priorities: OrchestrateIssuePriority[];
  assigneeIds: string[];
  projectIds: string[];
  search: string;
};

export type IssueSortField = "status" | "priority" | "title" | "created" | "updated";
export type IssueSortDir = "asc" | "desc";
export type IssueGroupBy = "status" | "priority" | "assignee" | "project" | "parent" | "none";
export type IssueViewMode = "list" | "board";

export type IssuesState = {
  items: OrchestrateIssue[];
  filters: IssueFilterState;
  viewMode: IssueViewMode;
  sortField: IssueSortField;
  sortDir: IssueSortDir;
  groupBy: IssueGroupBy;
  nestingEnabled: boolean;
  isLoading: boolean;
};

export type DashboardData = {
  agentCount: number;
  runningCount: number;
  pausedCount: number;
  errorCount: number;
  tasksInProgress: number;
  openTasks: number;
  blockedTasks: number;
  monthSpendCents: number;
  budgetCents: number;
  pendingApprovals: number;
};

// --- Slice state & actions ---

export type OrchestrateSliceState = {
  orchestrate: {
    agentInstances: AgentInstance[];
    skills: Skill[];
    projects: Project[];
    approvals: Approval[];
    activity: ActivityEntry[];
    costSummary: CostSummary | null;
    budgetPolicies: BudgetPolicy[];
    routines: Routine[];
    inboxItems: InboxItem[];
    inboxCount: number;
    wakeups: WakeupEntry[];
    dashboard: DashboardData | null;
    issues: IssuesState;
    isLoading: boolean;
  };
};

export type OrchestrateSliceActions = {
  setAgentInstances: (agents: AgentInstance[]) => void;
  addAgentInstance: (agent: AgentInstance) => void;
  updateAgentInstance: (id: string, patch: Partial<AgentInstance>) => void;
  removeAgentInstance: (id: string) => void;
  setSkills: (skills: Skill[]) => void;
  addSkill: (skill: Skill) => void;
  updateSkill: (id: string, patch: Partial<Skill>) => void;
  removeSkill: (id: string) => void;
  setProjects: (projects: Project[]) => void;
  addProject: (project: Project) => void;
  updateProject: (id: string, patch: Partial<Project>) => void;
  removeProject: (id: string) => void;
  setApprovals: (approvals: Approval[]) => void;
  setActivity: (entries: ActivityEntry[]) => void;
  setCostSummary: (summary: CostSummary | null) => void;
  setBudgetPolicies: (policies: BudgetPolicy[]) => void;
  setRoutines: (routines: Routine[]) => void;
  setInboxItems: (items: InboxItem[]) => void;
  setInboxCount: (count: number) => void;
  setWakeups: (wakeups: WakeupEntry[]) => void;
  setDashboard: (data: DashboardData | null) => void;
  setIssues: (issues: OrchestrateIssue[]) => void;
  setIssueFilters: (filters: Partial<IssueFilterState>) => void;
  setIssueViewMode: (mode: IssueViewMode) => void;
  setIssueSortField: (field: IssueSortField) => void;
  setIssueSortDir: (dir: IssueSortDir) => void;
  setIssueGroupBy: (groupBy: IssueGroupBy) => void;
  toggleNesting: () => void;
  setIssuesLoading: (loading: boolean) => void;
  setOrchestrateLoading: (loading: boolean) => void;
};

export type OrchestrateSlice = OrchestrateSliceState & OrchestrateSliceActions;

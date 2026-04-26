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
    isLoading: boolean;
  };
};

export type OrchestrateSliceActions = {
  setAgentInstances: (agents: AgentInstance[]) => void;
  setSkills: (skills: Skill[]) => void;
  setProjects: (projects: Project[]) => void;
  setApprovals: (approvals: Approval[]) => void;
  setActivity: (entries: ActivityEntry[]) => void;
  setCostSummary: (summary: CostSummary | null) => void;
  setBudgetPolicies: (policies: BudgetPolicy[]) => void;
  setRoutines: (routines: Routine[]) => void;
  setInboxItems: (items: InboxItem[]) => void;
  setInboxCount: (count: number) => void;
  setWakeups: (wakeups: WakeupEntry[]) => void;
  setDashboard: (data: DashboardData | null) => void;
  setOrchestrateLoading: (loading: boolean) => void;
};

export type OrchestrateSlice = OrchestrateSliceState & OrchestrateSliceActions;

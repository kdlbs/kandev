export type LinearAuthMethod = "api_key";

export interface LinearConfig {
  authMethod: LinearAuthMethod;
  defaultTeamKey: string;
  hasSecret: boolean;
  /** Captured from the most recent successful probe; used to build canonical URLs. */
  orgSlug?: string;
  /** Last time the backend probed credentials, or null if never probed. */
  lastCheckedAt?: string | null;
  /** Whether the most recent backend probe succeeded. */
  lastOk: boolean;
  /** Error message from the most recent failed probe; empty when ok or unprobed. */
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SetLinearConfigRequest {
  authMethod: LinearAuthMethod;
  defaultTeamKey?: string;
  secret?: string;
}

export interface TestLinearConnectionResult {
  ok: boolean;
  userId?: string;
  displayName?: string;
  email?: string;
  orgSlug?: string;
  orgName?: string;
  error?: string;
}

export interface LinearWorkflowState {
  id: string;
  name: string;
  /** backlog | unstarted | started | completed | canceled | triage */
  type: string;
  color?: string;
  position: number;
}

/** Three-bucket category Kandev uses across integrations to style status pills. */
export type LinearStateCategory = "new" | "indeterminate" | "done" | "";

export interface LinearIssue {
  id: string;
  /** Human identifier, e.g. "ENG-123". */
  identifier: string;
  title: string;
  description: string;
  stateId: string;
  stateName: string;
  stateType: string;
  stateCategory: LinearStateCategory;
  teamId: string;
  teamKey: string;
  /** 0=none, 1=urgent, 2=high, 3=medium, 4=low. */
  priority: number;
  priorityLabel?: string;
  assigneeName?: string;
  assigneeEmail?: string;
  assigneeIcon?: string;
  creatorName?: string;
  creatorIcon?: string;
  updated?: string;
  url: string;
  states: LinearWorkflowState[];
}

export interface LinearTeam {
  id: string;
  key: string;
  name: string;
}

export interface LinearSearchFilter {
  query?: string;
  teamKey?: string;
  stateIds?: string[];
  /** "me" | "unassigned" | "" (any) */
  assigned?: string;
}

export interface LinearSearchResult {
  issues: LinearIssue[];
  maxResults: number;
  isLast: boolean;
  nextPageToken?: string;
}

/**
 * A workspace-scoped Linear poller. The backend re-evaluates the structured
 * filter on `pollIntervalSeconds` cadence and creates a Kandev task in the
 * configured workflow step for each newly-matching issue.
 */
export interface LinearIssueWatch {
  id: string;
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  filter: LinearSearchFilter;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollIntervalSeconds: number;
  /** Last poll timestamp, or null when the watch has never run. */
  lastPolledAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface CreateLinearIssueWatchInput {
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  filter: LinearSearchFilter;
  agentProfileId?: string;
  executorProfileId?: string;
  prompt?: string;
  pollIntervalSeconds?: number;
  enabled?: boolean;
}

/** Patch shape: every field is optional so the UI can change one knob at a time. */
export interface UpdateLinearIssueWatchInput {
  workflowId?: string;
  workflowStepId?: string;
  filter?: LinearSearchFilter;
  agentProfileId?: string;
  executorProfileId?: string;
  prompt?: string;
  enabled?: boolean;
  pollIntervalSeconds?: number;
}

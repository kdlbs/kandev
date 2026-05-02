export type JiraAuthMethod = "api_token" | "session_cookie";

export interface JiraConfig {
  siteUrl: string;
  email: string;
  authMethod: JiraAuthMethod;
  defaultProjectKey: string;
  hasSecret: boolean;
  /** ISO timestamp when the session cookie's JWT expires, or null for api_token / opaque cookies. */
  secretExpiresAt?: string | null;
  /** Last time the backend probed credentials, or null if never probed. */
  lastCheckedAt?: string | null;
  /** Whether the most recent backend probe succeeded. */
  lastOk: boolean;
  /** Error message from the most recent failed probe; empty when ok or unprobed. */
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SetJiraConfigRequest {
  siteUrl: string;
  email: string;
  authMethod: JiraAuthMethod;
  defaultProjectKey?: string;
  secret?: string;
}

export interface TestJiraConnectionResult {
  ok: boolean;
  accountId?: string;
  displayName?: string;
  email?: string;
  error?: string;
}

export interface JiraTransition {
  id: string;
  name: string;
  toStatusId: string;
  toStatusName: string;
}

export type JiraStatusCategory = "new" | "indeterminate" | "done" | "";

export interface JiraTicket {
  key: string;
  summary: string;
  description: string;
  statusId: string;
  statusName: string;
  statusCategory: JiraStatusCategory;
  projectKey: string;
  issueType: string;
  issueTypeIcon?: string;
  priority?: string;
  priorityIcon?: string;
  assigneeName?: string;
  assigneeAvatar?: string;
  reporterName?: string;
  reporterAvatar?: string;
  updated?: string;
  url: string;
  transitions: JiraTransition[];
  fields?: Record<string, string>;
}

export interface JiraProject {
  key: string;
  name: string;
  id: string;
}

export interface JiraSearchResult {
  tickets: JiraTicket[];
  maxResults: number;
  isLast: boolean;
  nextPageToken?: string;
}

/**
 * A workspace-scoped JQL poller. The backend re-evaluates the JQL on
 * `pollIntervalSeconds` cadence and creates a Kandev task in the configured
 * workflow step for each newly-matching ticket.
 */
export interface JiraIssueWatch {
  id: string;
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  jql: string;
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

export interface CreateJiraIssueWatchInput {
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  jql: string;
  agentProfileId?: string;
  executorProfileId?: string;
  prompt?: string;
  pollIntervalSeconds?: number;
  enabled?: boolean;
}

/** Patch shape: every field is optional so the UI can change one knob at a time. */
export interface UpdateJiraIssueWatchInput {
  workflowId?: string;
  workflowStepId?: string;
  jql?: string;
  agentProfileId?: string;
  executorProfileId?: string;
  prompt?: string;
  enabled?: boolean;
  pollIntervalSeconds?: number;
}

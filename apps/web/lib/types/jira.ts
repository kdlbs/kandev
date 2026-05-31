/**
 * Authentication methods supported by the Jira integration.
 * - `api_token` — Atlassian Cloud only (Basic auth with email + token from id.atlassian.com).
 * - `pat` — Jira Server / Data Center only (Personal Access Token, sent as Bearer).
 * - `session_cookie` — works on both Cloud and Server (wraps the session JWT cookie).
 */
export type JiraAuthMethod = "api_token" | "pat" | "session_cookie";

/**
 * Jira deployment kind. Cloud uses REST v3 and the token-paginated search
 * endpoint; Server / Data Center expose only REST v2 with legacy `startAt`
 * pagination. The backend client picks endpoints based on this field.
 */
export type JiraInstanceType = "cloud" | "server";

export interface JiraConfig {
  siteUrl: string;
  email: string;
  authMethod: JiraAuthMethod;
  instanceType: JiraInstanceType;
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
  instanceType: JiraInstanceType;
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
  /**
   * Cap on concurrent open watcher-created tasks for this watch.
   * `null`/omitted means uncapped. Positive integers are accepted; the backend
   * rejects values ≤ 0.
   */
  maxInflightTasks?: number | null;
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
  /** Per-watch throttle cap; null = uncapped, positive int = cap. */
  maxInflightTasks?: number | null;
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
  /** Per-watch throttle cap; null = uncapped, positive int = cap. */
  maxInflightTasks?: number | null;
}

export type JiraAuthMethod = "api_token" | "session_cookie";

export interface JiraConfig {
  workspaceId: string;
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
  workspaceId: string;
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
  total: number;
  startAt: number;
  maxResults: number;
}

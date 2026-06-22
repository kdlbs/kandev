export type SentryAuthMethod = "auth_token";

export const SENTRY_AUTH_METHOD: SentryAuthMethod = "auth_token";

// SENTRY_DEFAULT_URL is the SaaS base URL pre-filled in the settings form and
// used by the backend when no custom instance URL is configured. Self-hosted
// installs replace it with their own host.
export const SENTRY_DEFAULT_URL = "https://sentry.io";

export interface SentryConfig {
  id: string;
  name: string;
  authMethod: SentryAuthMethod;
  url: string;
  hasSecret: boolean;
  lastCheckedAt?: string | null;
  lastOk: boolean;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SetSentryConfigRequest {
  name: string;
  authMethod: SentryAuthMethod;
  url: string;
  secret: string;
}

export interface TestSentryConnectionResult {
  ok: boolean;
  userId?: string;
  displayName?: string;
  email?: string;
  error?: string;
}

export interface SentryOrganization {
  id: string;
  slug: string;
  name: string;
}

export interface SentryProject {
  id: string;
  slug: string;
  name: string;
  orgSlug: string;
}

export type SentryLevel = "fatal" | "error" | "warning" | "info" | "debug";

export type SentryStatus = "unresolved" | "resolved" | "ignored";

export interface SentryIssue {
  id: string;
  shortId: string;
  title: string;
  culprit?: string;
  permalink: string;
  projectSlug: string;
  projectName?: string;
  level: SentryLevel;
  status: SentryStatus;
  count?: string;
  userCount?: number;
  firstSeen?: string;
  lastSeen?: string;
  assigneeName?: string;
}

export interface SentrySearchFilter {
  orgSlug: string;
  projectSlug?: string;
  environment?: string;
  levels?: SentryLevel[];
  statuses?: SentryStatus[];
  query?: string;
  statsPeriod?: string;
}

export interface SentrySearchResult {
  issues: SentryIssue[];
  nextPageToken?: string;
  isLast: boolean;
}

export interface SentryIssueWatch {
  id: string;
  workspaceId: string;
  instanceId: string;
  workflowId: string;
  workflowStepId: string;
  filter: SentrySearchFilter;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollIntervalSeconds: number;
  maxInflightTasks?: number | null;
  lastPolledAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateSentryIssueWatchRequest {
  workspaceId: string;
  instanceId: string;
  workflowId: string;
  workflowStepId: string;
  filter: SentrySearchFilter;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  pollIntervalSeconds: number;
  maxInflightTasks?: number | null;
  enabled?: boolean;
}

export interface UpdateSentryIssueWatchRequest {
  instanceId?: string;
  workflowId?: string;
  workflowStepId?: string;
  filter?: SentrySearchFilter;
  agentProfileId?: string;
  executorProfileId?: string;
  prompt?: string;
  enabled?: boolean;
  pollIntervalSeconds?: number;
  maxInflightTasks?: number | null;
}

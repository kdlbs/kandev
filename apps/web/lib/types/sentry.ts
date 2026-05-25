export type SentryAuthMethod = "auth_token";

export const SENTRY_AUTH_METHOD: SentryAuthMethod = "auth_token";

export interface SentryConfig {
  authMethod: string;
  defaultOrgSlug: string;
  defaultProjectSlug: string;
  hasSecret: boolean;
  lastCheckedAt?: string | null;
  lastOk: boolean;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SetSentryConfigRequest {
  authMethod: string;
  defaultOrgSlug: string;
  defaultProjectSlug: string;
  secret: string;
}

export interface TestSentryConnectionResult {
  ok: boolean;
  userId?: string;
  displayName?: string;
  email?: string;
  error?: string;
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
  levels?: string[];
  statuses?: string[];
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
  workflowId: string;
  workflowStepId: string;
  filter: SentrySearchFilter;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollIntervalSeconds: number;
  lastPolledAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateSentryIssueWatchRequest {
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  filter: SentrySearchFilter;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  pollIntervalSeconds: number;
  enabled?: boolean;
}

export interface UpdateSentryIssueWatchRequest {
  workflowId?: string;
  workflowStepId?: string;
  filter?: SentrySearchFilter;
  agentProfileId?: string;
  executorProfileId?: string;
  prompt?: string;
  enabled?: boolean;
  pollIntervalSeconds?: number;
}

export type SentryAuthMethod = "auth_token";

export const SENTRY_AUTH_METHOD: SentryAuthMethod = "auth_token";

// SENTRY_DEFAULT_URL is the SaaS base URL pre-filled in the settings form and
// used by the backend when no custom instance URL is configured. Self-hosted
// installs replace it with their own host.
export const SENTRY_DEFAULT_URL = "https://sentry.io";

export interface SentryConfig {
  // ID is the instance UUID: stable for the life of the instance, used as the
  // secret key, client-cache key, and issue-watch foreign key.
  id: string;
  workspaceId: string;
  // Name is the user-facing label, required and unique within a workspace.
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

// CreateSentryConfigRequest creates a new named Sentry instance in a workspace.
export interface CreateSentryConfigRequest {
  workspaceId: string;
  name: string;
  authMethod: SentryAuthMethod;
  url: string;
  secret: string;
}

// UpdateSentryConfigRequest updates an existing instance by ID. A blank secret
// keeps the stored value; a non-empty secret replaces it. Name/URL/AuthMethod
// always replace the stored values.
export interface UpdateSentryConfigRequest {
  name: string;
  authMethod: SentryAuthMethod;
  url: string;
  secret: string;
}

// CopySentryConfigRequest copies every instance from the source workspace into
// the target workspace: fresh IDs, secrets copied under new keys, names deduped,
// no watches carried over.
export interface CopySentryConfigRequest {
  sourceWorkspaceId: string;
  targetWorkspaceId: string;
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
  /**
   * The Sentry instance this watch polls. Immutable after create — changing it
   * means creating a new watch. Empty string = legacy unbound watch (resolved
   * to the workspace's sole instance at poll time).
   */
  sentryInstanceId: string;
  workflowId: string;
  workflowStepId: string;
  /**
   * Optional repository binding. Empty string = unbound: watcher-created tasks
   * launch in a blank scratch checkout (historical behaviour). When set, tasks
   * launch in an isolated worktree of this repository cut from `baseBranch`.
   */
  repositoryId: string;
  /** Branch the per-task worktree is cut from; empty = the repo's default. */
  baseBranch: string;
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
  /**
   * The Sentry instance to poll. Required and must belong to `workspaceId`.
   * Immutable after creation.
   */
  sentryInstanceId: string;
  workflowId: string;
  workflowStepId: string;
  /** Optional repository binding; empty/omitted = unbound (repo-less task). */
  repositoryId?: string;
  /** Base branch for the worktree; empty defaults to the repo's default branch. */
  baseBranch?: string;
  filter: SentrySearchFilter;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  pollIntervalSeconds: number;
  maxInflightTasks?: number | null;
  enabled?: boolean;
}

export interface UpdateSentryIssueWatchRequest {
  workflowId?: string;
  workflowStepId?: string;
  repositoryId?: string;
  baseBranch?: string;
  filter?: SentrySearchFilter;
  agentProfileId?: string;
  executorProfileId?: string;
  prompt?: string;
  enabled?: boolean;
  pollIntervalSeconds?: number;
  maxInflightTasks?: number | null;
}

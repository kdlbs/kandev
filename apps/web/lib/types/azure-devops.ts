export type AzureDevOpsAuthMethod = "pat";

export type AzureDevOpsConfig = {
  workspaceId: string;
  organizationUrl: string;
  defaultProjectId?: string;
  defaultProjectName?: string;
  authMethod: AzureDevOpsAuthMethod;
  hasSecret: boolean;
  lastCheckedAt?: string | null;
  lastOk: boolean;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
};

export type SetAzureDevOpsConfigRequest = {
  organizationUrl: string;
  defaultProjectId?: string;
  defaultProjectName?: string;
  authMethod: AzureDevOpsAuthMethod;
  pat?: string;
};

export type TestAzureDevOpsConnectionResult = {
  ok: boolean;
  id?: string;
  displayName?: string;
  email?: string;
  error?: string;
};

export type AzureDevOpsProject = {
  id: string;
  name: string;
  url: string;
};

export type AzureDevOpsRepository = {
  id: string;
  name: string;
  projectId: string;
  projectName: string;
  defaultBranch: string;
  webUrl: string;
};

export type AzureDevOpsSavedView = {
  id: string;
  kind: "work_item" | "pull_request";
  label: string;
  projectId: string;
  repositoryId?: string;
  wiql?: string;
  top?: number;
  status?: string;
  creator?: string;
  reviewer?: string;
  createdAt: string;
};

export type AzureDevOpsIdentity = {
  id: string;
  displayName: string;
  uniqueName?: string;
};

export type AzureDevOpsWorkItem = {
  id: number;
  revision: number;
  title: string;
  description?: string;
  state: string;
  type: string;
  project?: string;
  areaPath?: string;
  assignedTo?: string;
  tags?: string[];
  webUrl?: string;
  apiUrl?: string;
  fields?: Record<string, unknown>;
};

export type AzureDevOpsPlanningField = {
  referenceName: string;
  label: string;
  value: string;
};

export type AzureDevOpsWorkItemDetail = AzureDevOpsWorkItem & {
  planningFields: AzureDevOpsPlanningField[];
};

export type AzureDevOpsWorkItemComment = {
  id: number;
  content: string;
  author: AzureDevOpsIdentity;
  publishedAt?: string;
  updatedAt?: string;
};

export type AzureDevOpsWorkItemCommentPage = {
  comments: AzureDevOpsWorkItemComment[];
  continuationToken?: string;
};

export type AzureDevOpsWorkItemAssignmentUpdate = {
  revision: number;
  assigneeAction: "assign_current_user" | "unassign";
};

export type AzureDevOpsTeam = {
  id: string;
  name: string;
  projectId: string;
  projectName?: string;
};

export type AzureDevOpsBoardReference = { id: string; name: string };

export type AzureDevOpsBoardColumn = {
  id: string;
  name: string;
  columnType?: string;
  description?: string;
  isSplit?: boolean;
  itemLimit?: number;
  stateMappings?: Record<string, string>;
};

export type AzureDevOpsBoard = {
  id: string;
  name: string;
  columns: AzureDevOpsBoardColumn[];
  fields: {
    columnField: { referenceName: string };
    doneField: { referenceName: string };
    rowField: { referenceName: string };
  };
  rows?: Array<{ id: string; name: string; color?: string }>;
};

export type AzureDevOpsBoardWorkItem = AzureDevOpsWorkItem & {
  columnId: string;
  columnDone: boolean;
};

export type AzureDevOpsBoardSnapshot = {
  board: AzureDevOpsBoard;
  items: AzureDevOpsBoardWorkItem[];
};

export type AzureDevOpsBoardWorkItemUpdate = {
  revision: number;
  assigneeAction?: "assign_current_user" | "unassign";
  columnId?: string;
  columnDone?: boolean;
};

export type AzureDevOpsWorkItemSearchResult = {
  items: AzureDevOpsWorkItem[];
  count: number;
};

export type AzureDevOpsPullRequest = {
  id: number;
  title: string;
  description?: string;
  status: string;
  isDraft: boolean;
  sourceBranch: string;
  targetBranch: string;
  mergeStatus?: string;
  creationDate?: string;
  closedDate?: string;
  author: AzureDevOpsIdentity;
  projectId: string;
  projectName: string;
  repositoryId: string;
  repositoryName: string;
  webUrl: string;
  apiUrl: string;
};

export type AzureDevOpsPullRequestPage = {
  items: AzureDevOpsPullRequest[];
  count: number;
  skip: number;
  top: number;
};

export type AzureDevOpsReviewer = AzureDevOpsIdentity & {
  vote: number;
  isRequired: boolean;
  hasDeclined: boolean;
};

export type AzureDevOpsComment = {
  id: number;
  content: string;
  author: AzureDevOpsIdentity;
  commentType: string;
  publishedAt?: string;
  updatedAt?: string;
};

export type AzureDevOpsThread = {
  id: number;
  status: string;
  comments: AzureDevOpsComment[];
};

export type AzureDevOpsWorkItemRef = { id: number; url: string };

export type AzureDevOpsPolicyEvaluation = {
  id: string;
  status: string;
  name: string;
  isBlocking: boolean;
};

export type AzureDevOpsPullRequestFeedback = {
  pullRequest: AzureDevOpsPullRequest;
  reviewers: AzureDevOpsReviewer[];
  threads: AzureDevOpsThread[];
  linkedWorkItems: AzureDevOpsWorkItemRef[];
  policies: AzureDevOpsPolicyEvaluation[];
  reviewState: "approved" | "rejected" | "waiting" | "";
  policyState: "success" | "pending" | "failure" | "";
};

export type AzureDevOpsTaskPullRequest = {
  id: string;
  taskId: string;
  repositoryId: string;
  organizationUrl: string;
  projectId: string;
  azureRepositoryId: string;
  pullRequestId: number;
  pullRequestUrl: string;
  title: string;
  sourceBranch: string;
  targetBranch: string;
  authorId: string;
  authorName: string;
  status: string;
  reviewState?: string;
  policyState?: string;
  isDraft: boolean;
  lastSyncedAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type AssociateAzureDevOpsPullRequestRequest = {
  repositoryId: string;
  pullRequestId: number;
};

export type AzureDevOpsTaskWorkItem = {
  id: string;
  taskId: string;
  workspaceId: string;
  projectId: string;
  workItemId: number;
  workItemUrl: string;
  title: string;
  state: string;
  type: string;
  createdAt: string;
  updatedAt: string;
};

export type AssociateAzureDevOpsWorkItemRequest = {
  projectId: string;
  workItemId: number;
};

export type AzureDevOpsQueryPreset = {
  id: string;
  label: string;
  group: string;
  filters: Record<string, unknown>;
};

export type AzureDevOpsActionPreset = {
  id: string;
  label: string;
  hint: string;
  icon: string;
  promptTemplate: string;
};

export type AzureDevOpsWorkspaceSettings = {
  workspaceId: string;
  workItemQueries: AzureDevOpsQueryPreset[];
  pullRequestQueries: AzureDevOpsQueryPreset[];
  workItemActions: AzureDevOpsActionPreset[];
  pullRequestActions: AzureDevOpsActionPreset[];
};

export type UpdateAzureDevOpsWorkspaceSettingsRequest = {
  workItemQueries?: AzureDevOpsQueryPreset[] | null;
  pullRequestQueries?: AzureDevOpsQueryPreset[] | null;
  workItemActions?: AzureDevOpsActionPreset[] | null;
  pullRequestActions?: AzureDevOpsActionPreset[] | null;
};

export type AzureDevOpsCleanupPolicy = "auto" | "always" | "never";

export type AzureDevOpsWorkItemWatch = {
  id: string;
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  projectId: string;
  wiql: string;
  repositoryId: string;
  baseBranch: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollIntervalSeconds: number;
  cleanupPolicy: AzureDevOpsCleanupPolicy;
  maxInflightTasks?: number;
  lastError?: string;
  lastErrorAt?: string | null;
  lastPolledAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type AzureDevOpsPullRequestWatch = {
  id: string;
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  projectId: string;
  azureRepositoryId?: string;
  status: string;
  creatorId?: string;
  reviewerId?: string;
  repositoryId: string;
  baseBranch: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollIntervalSeconds: number;
  cleanupPolicy: AzureDevOpsCleanupPolicy;
  maxInflightTasks?: number;
  lastError?: string;
  lastErrorAt?: string | null;
  lastPolledAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type AzureDevOpsWorkItemWatchInput = Omit<
  AzureDevOpsWorkItemWatch,
  | "id"
  | "workspaceId"
  | "enabled"
  | "lastError"
  | "lastErrorAt"
  | "lastPolledAt"
  | "createdAt"
  | "updatedAt"
>;

export type AzureDevOpsPullRequestWatchInput = Omit<
  AzureDevOpsPullRequestWatch,
  | "id"
  | "workspaceId"
  | "enabled"
  | "lastError"
  | "lastErrorAt"
  | "lastPolledAt"
  | "createdAt"
  | "updatedAt"
>;

export type AzureDevOpsWatchResetResult = { generation: number; taskCount: number };

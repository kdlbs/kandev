import type { AzureDevOpsTaskPullRequest, AzureDevOpsTaskWorkItem } from "@/lib/types/azure-devops";

export type AzureDevOpsTaskPullRequestsState = {
  byTaskId: Record<string, AzureDevOpsTaskPullRequest[]>;
};

export type AzureDevOpsTaskWorkItemsState = {
  byTaskId: Record<string, AzureDevOpsTaskWorkItem[]>;
};

export type AzureDevOpsSliceState = {
  azureDevOpsTaskPullRequests: AzureDevOpsTaskPullRequestsState;
  azureDevOpsTaskWorkItems: AzureDevOpsTaskWorkItemsState;
};

export type AzureDevOpsSliceActions = {
  setAzureDevOpsTaskPullRequests: (
    pullRequests: Record<string, AzureDevOpsTaskPullRequest[]>,
  ) => void;
  setAzureDevOpsTaskPullRequest: (taskId: string, pullRequest: AzureDevOpsTaskPullRequest) => void;
  resetAzureDevOpsTaskPullRequests: () => void;
  setAzureDevOpsTaskWorkItems: (workItems: Record<string, AzureDevOpsTaskWorkItem[]>) => void;
  setAzureDevOpsTaskWorkItem: (taskId: string, workItem: AzureDevOpsTaskWorkItem) => void;
  resetAzureDevOpsTaskWorkItems: () => void;
};

export type AzureDevOpsSlice = AzureDevOpsSliceState & AzureDevOpsSliceActions;

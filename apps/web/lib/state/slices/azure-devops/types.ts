import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";

export type AzureDevOpsTaskPullRequestsState = {
  byTaskId: Record<string, AzureDevOpsTaskPullRequest[]>;
};

export type AzureDevOpsSliceState = {
  azureDevOpsTaskPullRequests: AzureDevOpsTaskPullRequestsState;
};

export type AzureDevOpsSliceActions = {
  setAzureDevOpsTaskPullRequests: (
    pullRequests: Record<string, AzureDevOpsTaskPullRequest[]>,
  ) => void;
  setAzureDevOpsTaskPullRequest: (taskId: string, pullRequest: AzureDevOpsTaskPullRequest) => void;
  resetAzureDevOpsTaskPullRequests: () => void;
};

export type AzureDevOpsSlice = AzureDevOpsSliceState & AzureDevOpsSliceActions;

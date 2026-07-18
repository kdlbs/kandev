import type { StateCreator } from "zustand";
import type { AzureDevOpsSlice, AzureDevOpsSliceState } from "./types";

export const defaultAzureDevOpsState: AzureDevOpsSliceState = {
  azureDevOpsTaskPullRequests: { byTaskId: {} },
};

type AzureDevOpsSliceCreator = StateCreator<
  AzureDevOpsSlice,
  [["zustand/immer", never]],
  [],
  AzureDevOpsSlice
>;

export const createAzureDevOpsSlice: AzureDevOpsSliceCreator = (set) => ({
  ...defaultAzureDevOpsState,
  setAzureDevOpsTaskPullRequests: (pullRequests) =>
    set((draft) => {
      draft.azureDevOpsTaskPullRequests.byTaskId = pullRequests;
    }),
  setAzureDevOpsTaskPullRequest: (taskId, pullRequest) =>
    set((draft) => {
      const existing = draft.azureDevOpsTaskPullRequests.byTaskId[taskId] ?? [];
      const index = existing.findIndex((item) => item.id === pullRequest.id);
      if (index >= 0) existing[index] = pullRequest;
      else existing.push(pullRequest);
      draft.azureDevOpsTaskPullRequests.byTaskId[taskId] = existing;
    }),
  resetAzureDevOpsTaskPullRequests: () =>
    set((draft) => {
      draft.azureDevOpsTaskPullRequests.byTaskId = {};
    }),
});

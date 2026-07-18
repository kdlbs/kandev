import type { StateCreator } from "zustand";
import type { AzureDevOpsSlice, AzureDevOpsSliceState } from "./types";

export const defaultAzureDevOpsState: AzureDevOpsSliceState = {
  azureDevOpsTaskPullRequests: { byTaskId: {} },
};

type AzureDevOpsStateCreator = StateCreator<
  AzureDevOpsSlice,
  [["zustand/immer", never]],
  [],
  AzureDevOpsSlice
>;
type AzureDevOpsSliceCreator = (set: Parameters<AzureDevOpsStateCreator>[0]) => AzureDevOpsSlice;

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

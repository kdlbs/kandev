import type { LinearIssueWatch } from "@/lib/types/linear";

export type LinearIssueWatchesState = {
  items: LinearIssueWatch[];
  loaded: boolean;
  loading: boolean;
};

export type LinearSliceState = {
  linearIssueWatches: LinearIssueWatchesState;
};

export type LinearSliceActions = {
  setLinearIssueWatches: (watches: LinearIssueWatch[]) => void;
  setLinearIssueWatchesLoading: (loading: boolean) => void;
  addLinearIssueWatch: (watch: LinearIssueWatch) => void;
  updateLinearIssueWatch: (watch: LinearIssueWatch) => void;
  removeLinearIssueWatch: (id: string) => void;
  /**
   * Clears items AND `loaded` so the next fetch effect runs again. Distinct
   * from `setLinearIssueWatches([])`, which marks the empty list as loaded
   * and would prevent a refetch on workspace switch.
   */
  resetLinearIssueWatches: () => void;
};

export type LinearSlice = LinearSliceState & LinearSliceActions;

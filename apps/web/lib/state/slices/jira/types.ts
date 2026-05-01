import type { JiraIssueWatch } from "@/lib/types/jira";

export type JiraIssueWatchesState = {
  items: JiraIssueWatch[];
  loaded: boolean;
  loading: boolean;
};

export type JiraSliceState = {
  jiraIssueWatches: JiraIssueWatchesState;
};

export type JiraSliceActions = {
  setJiraIssueWatches: (watches: JiraIssueWatch[]) => void;
  setJiraIssueWatchesLoading: (loading: boolean) => void;
  addJiraIssueWatch: (watch: JiraIssueWatch) => void;
  updateJiraIssueWatch: (watch: JiraIssueWatch) => void;
  removeJiraIssueWatch: (id: string) => void;
  /**
   * Clears items AND `loaded` so the next fetch effect runs again. Distinct
   * from `setJiraIssueWatches([])`, which marks the empty list as loaded and
   * would prevent a refetch on workspace switch.
   */
  resetJiraIssueWatches: () => void;
};

export type JiraSlice = JiraSliceState & JiraSliceActions;

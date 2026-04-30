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
};

export type JiraSlice = JiraSliceState & JiraSliceActions;

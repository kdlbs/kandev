import type { Draft } from "immer";
import type { JiraSlice, JiraSliceState } from "./types";

export const defaultJiraState: JiraSliceState = {
  jiraIssueWatches: { items: [], loaded: false, loading: false },
};

type ImmerSet = (updater: (draft: Draft<JiraSlice>) => void) => void;

export const createJiraSlice = (set: ImmerSet): JiraSlice => ({
  ...defaultJiraState,
  setJiraIssueWatches: (watches) =>
    set((draft) => {
      draft.jiraIssueWatches.items = watches;
      draft.jiraIssueWatches.loaded = true;
    }),
  setJiraIssueWatchesLoading: (loading) =>
    set((draft) => {
      draft.jiraIssueWatches.loading = loading;
    }),
  addJiraIssueWatch: (watch) =>
    set((draft) => {
      draft.jiraIssueWatches.items.push(watch);
    }),
  updateJiraIssueWatch: (watch) =>
    set((draft) => {
      const idx = draft.jiraIssueWatches.items.findIndex((w) => w.id === watch.id);
      if (idx >= 0) {
        draft.jiraIssueWatches.items[idx] = watch;
      }
    }),
  removeJiraIssueWatch: (id) =>
    set((draft) => {
      draft.jiraIssueWatches.items = draft.jiraIssueWatches.items.filter((w) => w.id !== id);
    }),
  resetJiraIssueWatches: () =>
    set((draft) => {
      draft.jiraIssueWatches.items = [];
      draft.jiraIssueWatches.loaded = false;
    }),
});

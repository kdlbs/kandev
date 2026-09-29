import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type { PreviewFeedbackSlice, PreviewFeedbackSliceState } from "./types";

export const defaultPreviewFeedbackState: PreviewFeedbackSliceState = {
  previewFeedback: {
    byTaskId: {},
    loadedByTaskId: {},
    loadingByTaskId: {},
    errorByTaskId: {},
  },
};

type ImmerSet = Parameters<
  StateCreator<AppState, [["zustand/immer", never]], [], PreviewFeedbackSlice>
>[0];

export const createPreviewFeedbackSlice = (set: ImmerSet): PreviewFeedbackSlice => ({
  ...defaultPreviewFeedbackState,

  setTaskPreviewFeedback: (taskId, snapshot) =>
    set((draft) => {
      const current = draft.previewFeedback.byTaskId[taskId];
      if (snapshot.task_id !== taskId || (current && snapshot.revision < current.revision)) return;
      draft.previewFeedback.byTaskId[taskId] = snapshot;
      draft.previewFeedback.loadedByTaskId[taskId] = true;
      draft.previewFeedback.loadingByTaskId[taskId] = false;
      delete draft.previewFeedback.errorByTaskId[taskId];
    }),

  setTaskPreviewFeedbackLoading: (taskId, loading) =>
    set((draft) => {
      draft.previewFeedback.loadingByTaskId[taskId] = loading;
    }),

  setTaskPreviewFeedbackError: (taskId, error) =>
    set((draft) => {
      draft.previewFeedback.loadingByTaskId[taskId] = false;
      if (error) draft.previewFeedback.errorByTaskId[taskId] = error;
      else delete draft.previewFeedback.errorByTaskId[taskId];
    }),

  clearTaskPreviewFeedbackState: (taskId) =>
    set((draft) => {
      delete draft.previewFeedback.byTaskId[taskId];
      delete draft.previewFeedback.loadedByTaskId[taskId];
      delete draft.previewFeedback.loadingByTaskId[taskId];
      delete draft.previewFeedback.errorByTaskId[taskId];
    }),
});

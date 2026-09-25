import type { TaskPreviewFeedbackSnapshot } from "@/lib/types/http";

export type PreviewFeedbackSliceState = {
  previewFeedback: {
    byTaskId: Record<string, TaskPreviewFeedbackSnapshot | undefined>;
    loadedByTaskId: Record<string, boolean | undefined>;
    loadingByTaskId: Record<string, boolean | undefined>;
    errorByTaskId: Record<string, string | undefined>;
  };
};

export type PreviewFeedbackSliceActions = {
  setTaskPreviewFeedback: (taskId: string, snapshot: TaskPreviewFeedbackSnapshot) => void;
  setTaskPreviewFeedbackLoading: (taskId: string, loading: boolean) => void;
  setTaskPreviewFeedbackError: (taskId: string, error?: string) => void;
  clearTaskPreviewFeedbackState: (taskId: string) => void;
};

export type PreviewFeedbackSlice = PreviewFeedbackSliceState & PreviewFeedbackSliceActions;

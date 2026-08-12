import type {
  TaskLspAction,
  TaskLspCapacity,
  TaskLspLanguageSnapshot,
  TaskLspSnapshot,
} from "@/lib/types/http-lsp";

export type TaskLspTaskState = {
  languages: Record<string, TaskLspLanguageSnapshot>;
  capacity: TaskLspCapacity;
  retiredCapacityEpochs?: string[];
  loaded: boolean;
  loading: boolean;
  error: string | null;
  errorEpoch?: number;
};

export type LspSliceState = {
  taskLsp: {
    byTaskId: Record<string, TaskLspTaskState>;
    pendingByKey: Record<string, TaskLspAction | undefined>;
  };
};

export type LspSliceActions = {
  setTaskLspSnapshot: (snapshot: TaskLspSnapshot, expectedErrorEpoch?: number) => void;
  mergeTaskLspLanguage: (snapshot: TaskLspLanguageSnapshot) => void;
  setTaskLspLoading: (taskId: string, loading: boolean) => void;
  setTaskLspError: (taskId: string, error: string | null) => void;
  setTaskLspActionPending: (taskId: string, language: string, action?: TaskLspAction) => void;
  clearTaskLsp: (taskId: string) => void;
};

export type LspSlice = LspSliceState & LspSliceActions;

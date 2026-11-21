import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

export type TaskAsyncQuestionsReadStatus = "idle" | "loading" | "ready" | "error";

export type TaskAsyncQuestionsScopeState = {
  questions: ClarificationInboxAsyncQuestion[];
  status: TaskAsyncQuestionsReadStatus;
  appliedGeneration: number;
};

export type TaskAsyncQuestionsSliceState = {
  taskAsyncQuestions: {
    byScopeKey: Record<string, TaskAsyncQuestionsScopeState>;
    generationByScopeKey: Record<string, number>;
  };
};

export type TaskAsyncQuestionsSliceActions = {
  beginTaskAsyncQuestionsRead: (scopeKey: string) => number;
  setTaskAsyncQuestionsPage: (
    scopeKey: string,
    generation: number,
    questions: ClarificationInboxAsyncQuestion[],
  ) => void;
  setTaskAsyncQuestionsError: (scopeKey: string, generation: number) => void;
};

export type TaskAsyncQuestionsSlice = TaskAsyncQuestionsSliceState & TaskAsyncQuestionsSliceActions;

import type { StateCreator } from "zustand";
import type { AppState } from "../../app-state-types";
import type {
  TaskAsyncQuestionsScopeState,
  TaskAsyncQuestionsSlice,
  TaskAsyncQuestionsSliceState,
} from "./types";

export const defaultTaskAsyncQuestionsState: TaskAsyncQuestionsSliceState = {
  taskAsyncQuestions: { byScopeKey: {}, generationByScopeKey: {} },
};

const emptyScopeState = (): TaskAsyncQuestionsScopeState => ({
  questions: [],
  status: "idle",
  appliedGeneration: 0,
});

type ImmerSet = Parameters<
  StateCreator<AppState, [["zustand/immer", never]], [], TaskAsyncQuestionsSlice>
>[0];

export const createTaskAsyncQuestionsSlice = (set: ImmerSet): TaskAsyncQuestionsSlice => ({
  ...defaultTaskAsyncQuestionsState,

  beginTaskAsyncQuestionsRead: (scopeKey) => {
    let generation = 0;
    set((draft) => {
      generation = (draft.taskAsyncQuestions.generationByScopeKey[scopeKey] ?? 0) + 1;
      draft.taskAsyncQuestions.generationByScopeKey[scopeKey] = generation;
      const scope = draft.taskAsyncQuestions.byScopeKey[scopeKey] ?? emptyScopeState();
      scope.status = "loading";
      draft.taskAsyncQuestions.byScopeKey[scopeKey] = scope;
    });
    return generation;
  },

  setTaskAsyncQuestionsPage: (scopeKey, generation, questions) =>
    set((draft) => {
      if (draft.taskAsyncQuestions.generationByScopeKey[scopeKey] !== generation) return;
      draft.taskAsyncQuestions.byScopeKey[scopeKey] = {
        questions,
        status: "ready",
        appliedGeneration: generation,
      };
    }),

  setTaskAsyncQuestionsError: (scopeKey, generation) =>
    set((draft) => {
      if (draft.taskAsyncQuestions.generationByScopeKey[scopeKey] !== generation) return;
      draft.taskAsyncQuestions.byScopeKey[scopeKey] = {
        ...emptyScopeState(),
        status: "error",
        appliedGeneration: generation,
      };
    }),
});

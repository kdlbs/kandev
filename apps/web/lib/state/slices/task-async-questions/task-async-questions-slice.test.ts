import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";
import { createTaskAsyncQuestionsSlice } from "./task-async-questions-slice";
import type { TaskAsyncQuestionsSlice } from "./types";

function newStore() {
  return create<TaskAsyncQuestionsSlice>()(
    immer((set) =>
      createTaskAsyncQuestionsSlice(
        set as unknown as Parameters<typeof createTaskAsyncQuestionsSlice>[0],
      ),
    ),
  );
}

describe("task async question slice", () => {
  it("keeps the last good questions after a transient refresh error", () => {
    const store = newStore();
    const question = { question_id: "q1" } as ClarificationInboxAsyncQuestion;
    const firstRead = store.getState().beginTaskAsyncQuestionsRead("scope");
    store.getState().setTaskAsyncQuestionsPage("scope", firstRead, [question]);

    const retry = store.getState().beginTaskAsyncQuestionsRead("scope");
    store.getState().setTaskAsyncQuestionsError("scope", retry);

    expect(store.getState().taskAsyncQuestions.byScopeKey.scope).toMatchObject({
      questions: [question],
      status: "error",
      appliedGeneration: retry,
    });
  });
});

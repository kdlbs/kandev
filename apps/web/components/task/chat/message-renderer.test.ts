import { describe, expect, it } from "vitest";
import { shouldShowDescriptionStartButton } from "./message-renderer";

const TASK_ID = "t1";
const SESSION_ID = "s1";
const CREATED_STATE = "CREATED";

describe("shouldShowDescriptionStartButton", () => {
  it("shows for a never-started (CREATED) session", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: CREATED_STATE,
        taskState: "CREATED",
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      }),
    ).toBe(true);
  });

  it("does NOT show for a resume-skipped session (the composer hint owns that surface)", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "WAITING_FOR_INPUT",
        taskState: "CREATED",
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      }),
    ).toBe(false);
  });

  it("does NOT show for a FAILED session (recovery owns the affordance)", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "FAILED",
        taskState: "CREATED",
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      }),
    ).toBe(false);
  });

  it("hides while the task is SCHEDULING", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: CREATED_STATE,
        taskState: "SCHEDULING",
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      }),
    ).toBe(false);
  });

  it("hides when no task/session context is bound", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: CREATED_STATE,
        taskState: "CREATED",
        taskId: undefined,
        sessionId: undefined,
      }),
    ).toBe(false);
  });
});

import { describe, expect, it } from "vitest";
import type { AppState } from "@/lib/state/store";
import { isTaskWorkspaceSearchAvailable } from "./task-workspace-search";

const TASK_ID = "task-1";
const SESSION_ID = "session-1";

function searchState({
  activeTaskId = TASK_ID,
  activeSessionId = SESSION_ID,
  sessionTaskId = TASK_ID,
}: {
  activeTaskId?: string | null;
  activeSessionId?: string | null;
  sessionTaskId?: string;
} = {}): AppState {
  return {
    tasks: {
      activeTaskId,
      activeSessionId,
    },
    taskSessions: {
      items: activeSessionId
        ? {
            [activeSessionId]: {
              id: activeSessionId,
              task_id: sessionTaskId,
            },
          }
        : {},
    },
  } as unknown as AppState;
}

describe("isTaskWorkspaceSearchAvailable", () => {
  it("allows search for the active session inside its task detail route", () => {
    expect(isTaskWorkspaceSearchAvailable(searchState(), `/t/${TASK_ID}`)).toBe(true);
  });

  it.each([
    ["missing active task", searchState({ activeTaskId: null }), `/t/${TASK_ID}`],
    ["missing active session", searchState({ activeSessionId: null }), `/t/${TASK_ID}`],
    ["non-task route", searchState(), "/tasks"],
    ["different task route", searchState(), "/t/task-2"],
    [
      "session belonging to another task",
      searchState({ sessionTaskId: "task-2" }),
      `/t/${TASK_ID}`,
    ],
  ])("rejects %s", (_label, state, pathname) => {
    expect(isTaskWorkspaceSearchAvailable(state, pathname)).toBe(false);
  });
});

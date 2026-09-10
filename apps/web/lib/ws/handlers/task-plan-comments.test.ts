import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerTaskPlanCommentsHandlers } from "./task-plan-comments";

function message(revision: number): BackendMessageMap["task.plan.comments.changed"] {
  return {
    id: "event-1",
    type: "notification",
    action: "task.plan.comments.changed",
    payload: {
      task_id: "task-1",
      plan_id: "plan-1",
      revision,
      comments: [],
    },
  };
}

describe("task plan comment WebSocket handler", () => {
  it("applies the complete snapshot through the task-plan store", () => {
    const setTaskPlanComments = vi.fn();
    const state = { setTaskPlanComments } as unknown as AppState;
    const store = { getState: () => state } as StoreApi<AppState>;

    registerTaskPlanCommentsHandlers(store)["task.plan.comments.changed"]!(message(3));

    expect(setTaskPlanComments).toHaveBeenCalledWith("task-1", message(3).payload);
  });
});

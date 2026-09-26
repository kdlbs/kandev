import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerPreviewFeedbackHandlers } from "./preview-feedback";

const message: BackendMessageMap["task.preview_feedback.changed"] = {
  id: "event-1",
  type: "notification",
  action: "task.preview_feedback.changed",
  payload: { task_id: "task-1", revision: 3, items: [] },
};

describe("preview feedback WebSocket handler", () => {
  it("applies the complete task snapshot", () => {
    const setTaskPreviewFeedback = vi.fn();
    const state = { setTaskPreviewFeedback } as unknown as AppState;
    const store = { getState: () => state } as StoreApi<AppState>;
    const handler = registerPreviewFeedbackHandlers(store)["task.preview_feedback.changed"];

    expect(handler).toBeTypeOf("function");
    handler?.(message);
    expect(setTaskPreviewFeedback).toHaveBeenCalledWith("task-1", message.payload);
  });
});

import { beforeEach, describe, expect, it, vi } from "vitest";

const request = vi.hoisted(() => vi.fn());
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request }),
}));

import * as previewApi from "./preview-feedback-api";

const TASK_ID = "task-1";
const FEEDBACK_ID = "feedback-1";

describe("preview feedback API", () => {
  beforeEach(() => request.mockReset());

  it("loads the task-owned collection", async () => {
    request.mockResolvedValue({ task_id: TASK_ID, revision: 0, items: [] });

    await previewApi.getTaskPreviewFeedback(TASK_ID);

    expect(request).toHaveBeenCalledWith("task.preview_feedback.list", { task_id: TASK_ID });
  });

  it("provides create, update, delete, and clear mutations", () => {
    const api = previewApi as unknown as Record<string, unknown>;

    expect(api.createTaskPreviewFeedback).toBeTypeOf("function");
    expect(api.updateTaskPreviewFeedback).toBeTypeOf("function");
    expect(api.deleteTaskPreviewFeedback).toBeTypeOf("function");
    expect(api.clearTaskPreviewFeedback).toBeTypeOf("function");
  });

  it("sends capture evidence and optimistic mutation guards", async () => {
    request.mockResolvedValue({ task_id: TASK_ID, revision: 1, items: [] });
    await previewApi.createTaskPreviewFeedback({
      taskId: TASK_ID,
      id: FEEDBACK_ID,
      kind: "text",
      comment: "Increase contrast",
      sourceKind: "browser",
      sourceLabel: "Local app",
      pageRoute: "/products",
      pageTitle: "Products",
      selectedText: "Choose a plan",
      textAnchor: { start: { node_path: [0], offset: 0 }, end: { node_path: [0], offset: 13 } },
    });
    expect(request).toHaveBeenLastCalledWith(
      "task.preview_feedback.create",
      expect.objectContaining({
        task_id: TASK_ID,
        selected_text: "Choose a plan",
        text_anchor: expect.any(Object),
      }),
    );

    await previewApi.updateTaskPreviewFeedback({
      taskId: TASK_ID,
      id: FEEDBACK_ID,
      comment: "Use stronger contrast",
      expectedVersion: 1,
    });
    expect(request).toHaveBeenLastCalledWith("task.preview_feedback.update", {
      task_id: TASK_ID,
      id: FEEDBACK_ID,
      comment: "Use stronger contrast",
      expected_version: 1,
    });

    await previewApi.deleteTaskPreviewFeedback({
      taskId: TASK_ID,
      id: FEEDBACK_ID,
      expectedVersion: 2,
    });
    await previewApi.clearTaskPreviewFeedback(TASK_ID, 3);
    expect(request).toHaveBeenLastCalledWith("task.preview_feedback.clear", {
      task_id: TASK_ID,
      expected_revision: 3,
    });
  });
});

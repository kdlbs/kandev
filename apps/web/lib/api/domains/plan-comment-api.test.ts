import { beforeEach, describe, expect, it, vi } from "vitest";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: getWebSocketClientMock }));

import {
  createTaskPlanComment,
  deleteTaskPlanComment,
  getTaskPlanComments,
  updateTaskPlanComment,
} from "./plan-comment-api";

describe("plan comment API", () => {
  beforeEach(() => getWebSocketClientMock.mockReset());

  it("uses task-keyed CRUD actions and version guards", async () => {
    const request = vi.fn().mockResolvedValue({
      task_id: "task-1",
      plan_id: "plan-1",
      revision: 1,
      comments: [],
    });
    getWebSocketClientMock.mockReturnValue({ request });

    await getTaskPlanComments("task-1");
    await createTaskPlanComment({
      taskId: "task-1",
      planId: "plan-1",
      id: "comment-1",
      body: "feedback",
      selectedText: "selection",
      anchorFrom: 3,
      anchorTo: 8,
    });
    await updateTaskPlanComment({
      taskId: "task-1",
      planId: "plan-1",
      id: "comment-1",
      body: "updated",
      expectedVersion: 2,
    });
    await deleteTaskPlanComment({
      taskId: "task-1",
      planId: "plan-1",
      id: "comment-1",
      expectedVersion: 3,
    });

    expect(request.mock.calls).toEqual([
      ["task.plan.comments.list", { task_id: "task-1" }],
      [
        "task.plan.comments.create",
        {
          task_id: "task-1",
          plan_id: "plan-1",
          id: "comment-1",
          body: "feedback",
          selected_text: "selection",
          anchor_from: 3,
          anchor_to: 8,
        },
      ],
      [
        "task.plan.comments.update",
        {
          task_id: "task-1",
          plan_id: "plan-1",
          id: "comment-1",
          body: "updated",
          expected_version: 2,
        },
      ],
      [
        "task.plan.comments.delete",
        {
          task_id: "task-1",
          plan_id: "plan-1",
          id: "comment-1",
          expected_version: 3,
        },
      ],
    ]);
  });
});

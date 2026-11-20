import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getBulkMoveApiError, bulkMoveSelectedTasks } from "./task-bulk-move-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("bulkMoveSelectedTasks", () => {
  it("returns moved task IDs from a successful batch", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ moved_count: 2, moved_task_ids: ["task-1", "task-2"] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(
      bulkMoveSelectedTasks(
        {
          task_ids: ["task-1", "task-2"],
          target_workflow_id: "workflow-2",
          target_step_id: "step-2",
        },
        { baseUrl: API_BASE_URL },
      ),
    ).resolves.toEqual({ moved_count: 2, moved_task_ids: ["task-1", "task-2"] });
  });

  it("parses structured refusal details from a conflict response", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: "bulk_move_refused",
          moved_task_ids: [],
          refusals: [
            {
              task_id: "task-1",
              task_title: "Fix login",
              code: "task_approvals_pending",
              message: "task approvals pending: CEO",
              pending_approvers: [{ agent_profile_id: "agent-ceo", name: "CEO" }],
            },
          ],
        }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );

    let caught: unknown;
    try {
      await bulkMoveSelectedTasks(
        { task_ids: ["task-1"], target_workflow_id: "workflow-2", target_step_id: "step-2" },
        { baseUrl: API_BASE_URL },
      );
    } catch (error) {
      caught = error;
    }

    expect(getBulkMoveApiError(caught)?.result).toMatchObject({
      code: "bulk_move_refused",
      moved_task_ids: [],
      refusals: [{ task_id: "task-1", task_title: "Fix login", code: "task_approvals_pending" }],
    });
  });
});

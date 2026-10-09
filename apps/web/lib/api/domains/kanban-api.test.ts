import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  attachTaskWorkspaceSources,
  detachTask,
  deleteTaskAfterUserAction,
  deleteTask,
  getTaskDeletePreflight,
  listTasksByWorkspace,
  querySidebarTasks,
  moveTask,
  previewWorkflowMove,
  reorderStepTasks,
  updateTask,
  updateTaskPortForwarding,
} from "./kanban-api";
import { ApiError } from "../client";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";
const CONFIRMATION_ID = "confirmation-1";
const SECOND_CONFIRMATION_ID = "confirmation-2";
const WORKFLOW_ID = "workflow-1";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("deleteTask", () => {
  it("sends cascade and discard consent independently", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await deleteTask(
      "task-1",
      {
        cascade: true,
        discardWorktreeChanges: true,
        confirmationId: CONFIRMATION_ID,
      },
      { baseUrl: API_BASE_URL },
    );

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(
      `${API_BASE_URL}/api/v1/tasks/task-1?cascade=true&discard_worktree_changes=true`,
    );
    expect(init?.method).toBe("DELETE");
    expect(new Headers(init?.headers).get("X-Kandev-Task-Delete-Confirmation")).toBe(
      CONFIRMATION_ID,
    );
  });
});

describe("moveTask completion override", () => {
  it("binds a human override to the selected task move", async () => {
    const taskId = "task-1";
    const workflowId = WORKFLOW_ID;
    const completionStepId = "step-done";
    const overrideReason = "Accepted by the release owner";
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ task: { id: taskId, workflow_step_id: completionStepId } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await moveTask(
      taskId,
      {
        workflow_id: workflowId,
        workflow_step_id: completionStepId,
        completion_override: { expected_revision: 4, reason: overrideReason },
      },
      { baseUrl: API_BASE_URL },
    );

    expect(fetchSpy.mock.calls[0]).toMatchObject([
      `${API_BASE_URL}/api/v1/tasks/${taskId}/move`,
      {
        method: "POST",
        body: JSON.stringify({
          workflow_id: workflowId,
          workflow_step_id: completionStepId,
          completion_override: { expected_revision: 4, reason: overrideReason },
        }),
      },
    ]);
  });
});

describe("getTaskDeletePreflight", () => {
  it("posts the exact scope and bypasses caches", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ requires_discard_consent: true, confirmation_id: CONFIRMATION_ID }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(
      getTaskDeletePreflight(["task-1", "task-2"], true, true, { baseUrl: API_BASE_URL }),
    ).resolves.toEqual({ requires_discard_consent: true, confirmation_id: CONFIRMATION_ID });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks/delete-preflight`);
    expect(init).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        task_ids: ["task-1", "task-2"],
        cascade: true,
        discard_worktree_changes: true,
      }),
      cache: "no-store",
    });
  });
});

describe("deleteTaskAfterUserAction", () => {
  it("records a fresh native preview before a confirmed app cleanup", async () => {
    fetchSpy
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            requires_discard_consent: false,
            confirmation_id: SECOND_CONFIRMATION_ID,
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(new Response(JSON.stringify({ success: true }), { status: 200 }));

    await deleteTaskAfterUserAction("task-1", { cascade: true }, { baseUrl: API_BASE_URL });

    expect(fetchSpy).toHaveBeenCalledTimes(2);
    const [, previewInit] = fetchSpy.mock.calls[0];
    expect(previewInit).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        task_ids: ["task-1"],
        cascade: true,
        discard_worktree_changes: false,
      }),
    });
    const [deleteUrl, deleteInit] = fetchSpy.mock.calls[1];
    expect(deleteUrl).toBe(`${API_BASE_URL}/api/v1/tasks/task-1?cascade=true`);
    expect(new Headers(deleteInit?.headers).get("X-Kandev-Task-Delete-Confirmation")).toBe(
      SECOND_CONFIRMATION_ID,
    );
  });
});

describe("detachTask", () => {
  it("posts without a body to the canonical detach endpoint", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "child-1", parent_id: "" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await detachTask("child-1", { baseUrl: API_BASE_URL });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks/child-1/detach`);
    expect(init?.method).toBe("POST");
    expect(init?.body).toBeUndefined();
  });
});

describe("updateTask", () => {
  it("carries priority as a partial-update field", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "task-1", priority: "critical" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await updateTask("task-1", { priority: "critical" }, { baseUrl: API_BASE_URL });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks/task-1`);
    expect(init).toMatchObject({
      method: "PATCH",
      body: JSON.stringify({ priority: "critical" }),
    });
  });
});

describe("reorderStepTasks", () => {
  it("PUTs the band and ordered ids to the hyphenated collection route", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          workflow_step_id: "step-1",
          revision: 1,
          tasks: [
            { id: "task-b", position: 0 },
            { id: "task-a", position: 1 },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const result = await reorderStepTasks(
      "step-1",
      { band: "admitted", ordered_task_ids: ["task-b", "task-a"] },
      { baseUrl: API_BASE_URL },
    );

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/workflow-steps/step-1/tasks/reorder`);
    expect(init?.method).toBe("PUT");
    expect(init?.body).toBe(
      JSON.stringify({ band: "admitted", ordered_task_ids: ["task-b", "task-a"] }),
    );
    expect(result.revision).toBe(1);
    expect(result.tasks).toEqual([
      { id: "task-b", position: 0 },
      { id: "task-a", position: 1 },
    ]);
  });

  it("surfaces a step_changed conflict as an ApiError carrying the authoritative order", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: "step_changed",
          workflow_step_id: "step-1",
          revision: 3,
          tasks: [{ id: "task-a", position: 0 }],
        }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(
      reorderStepTasks(
        "step-1",
        { band: "admitted", ordered_task_ids: ["task-a"] },
        { baseUrl: API_BASE_URL },
      ),
    ).rejects.toMatchObject({
      status: 409,
      body: { code: "step_changed", revision: 3 },
    });
  });

  it("surfaces an invalid_reorder rejection with no task list", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ code: "invalid_reorder" }), {
        status: 400,
        headers: { "Content-Type": "application/json" },
      }),
    );

    let caught: unknown;
    try {
      await reorderStepTasks(
        "step-1",
        { band: "admitted", ordered_task_ids: [] },
        { baseUrl: API_BASE_URL },
      );
    } catch (error) {
      caught = error;
    }
    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).status).toBe(400);
    expect((caught as ApiError).body).toEqual({ code: "invalid_reorder" });
  });
});

describe("updateTaskPortForwarding", () => {
  it("patches only the task-scoped preference endpoint", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "task-1", metadata: { port_forwarding_enabled: true } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(
      updateTaskPortForwarding("task-1", true, { baseUrl: API_BASE_URL }),
    ).resolves.toMatchObject({
      id: "task-1",
      metadata: { port_forwarding_enabled: true },
    });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks/task-1/port-forwarding`);
    expect(init).toMatchObject({
      method: "PATCH",
      body: JSON.stringify({ enabled: true }),
    });
    expect(new Headers(init?.headers).get("Content-Type")).toBe("application/json");
  });
});

describe("attachTaskWorkspaceSources", () => {
  it("posts the exact mixed-source payload and returns the persisted projection", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          task_id: "task-1",
          repositories: [],
          workspace_folders: [
            { id: "folder-1", local_path: "/docs", display_name: "docs", position: 0 },
          ],
          workspace_path: "/workspace/task-1",
          session_ids: ["session-1"],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(
      attachTaskWorkspaceSources(
        "task-1",
        { sources: [{ kind: "folder", local_path: "/docs", display_name: "docs" }] },
        { baseUrl: API_BASE_URL },
      ),
    ).resolves.toMatchObject({ task_id: "task-1", workspace_path: "/workspace/task-1" });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks/task-1/workspace-sources`);
    expect(init).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        sources: [{ kind: "folder", local_path: "/docs", display_name: "docs" }],
      }),
    });
    expect(new Headers(init?.headers).get("Content-Type")).toBe("application/json");
  });

  it("preserves normalized API errors for retry UI", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "task has an active turn" }), {
        status: 409,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(
      attachTaskWorkspaceSources("task-1", { sources: [] }, { baseUrl: API_BASE_URL }),
    ).rejects.toMatchObject({ name: "ApiError", status: 409, message: "task has an active turn" });
  });
});

describe("listTasksByWorkspace", () => {
  it("requests the archived-only mode without changing the existing list contract", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ tasks: [], total: 0 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await listTasksByWorkspace(
      "ws-1",
      { page: 2, pageSize: 100, onlyArchived: true, sort: "updated_desc" },
      { baseUrl: API_BASE_URL },
    );

    expect(fetchSpy).toHaveBeenCalledOnce();
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${API_BASE_URL}/api/v1/workspaces/ws-1/tasks?page=2&page_size=100&only_archived=true&sort=updated_desc`,
    );
  });
});

describe("querySidebarTasks", () => {
  it("posts the bounded complete-view query to the additive route", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          query_key: "query-key",
          page: 1,
          page_size: 100,
          total_entries: 2,
          total_tasks: 1,
          total_visible_tasks: 1,
          has_previous: false,
          has_next: false,
          entries: [{ kind: "task", task_id: "task-1", task: { id: "task-1" } }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    const query = {
      filters: [{ dimension: "archived", op: "is", value: true }],
      sort: { key: "updatedAt", direction: "desc" },
      group: "none",
      collapsed_group_keys: [],
      collapsed_task_ids: [],
      page: 1,
      page_size: 100,
      locale: "en",
    };

    await expect(
      querySidebarTasks("ws-1", query, { baseUrl: API_BASE_URL }),
    ).resolves.toMatchObject({
      query_key: "query-key",
      total_visible_tasks: 1,
    });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/workspaces/ws-1/sidebar/query`);
    expect(init).toMatchObject({ method: "POST", body: JSON.stringify(query) });
  });
});

describe("moveTask", () => {
  const workflowId = WORKFLOW_ID;
  const workflowStepId = "step-2";

  it("normalizes one-shot entry options and omits blank values", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ task: { id: "task-1" }, workflow_step: { id: workflowStepId } }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await moveTask(
      "task-1",
      {
        workflow_id: workflowId,
        workflow_step_id: workflowStepId,
        position: 0,
        entry_options: {
          reset_context: true,
          instructions: "  Start the verification pass.  ",
          skip_step_prompt: true,
        },
      },
      { baseUrl: API_BASE_URL },
    );

    const [, init] = fetchSpy.mock.calls[0];
    expect(init?.body).toBe(
      JSON.stringify({
        workflow_id: workflowId,
        workflow_step_id: workflowStepId,
        position: 0,
        entry_options: {
          reset_context: true,
          skip_step_prompt: true,
          instructions: "Start the verification pass.",
        },
      }),
    );
  });

  it("keeps destination-only move payloads unchanged", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ task: { id: "task-1" }, workflow_step: { id: workflowStepId } }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await moveTask(
      "task-1",
      { workflow_id: workflowId, workflow_step_id: workflowStepId, position: 0 },
      { baseUrl: API_BASE_URL },
    );

    const [, init] = fetchSpy.mock.calls[0];
    expect(init?.body).toBe(
      JSON.stringify({ workflow_id: workflowId, workflow_step_id: workflowStepId, position: 0 }),
    );
  });
});

describe("previewWorkflowMove", () => {
  it("posts normalized options with a no-store cache policy", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          task_id: "task-1",
          workflow_step_id: "step-2",
          evaluated_at: "2026-09-14T00:00:00Z",
          outcome: "create_new",
          model: {
            before: { known: false },
            after: { known: true, label: "gpt-5.6-luna" },
          },
          context_reset: false,
          context_reset_state: "unchanged",
          source_disposition: "park",
          dispatch: "prompt",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await previewWorkflowMove(
      "task-1",
      {
        workflow_id: WORKFLOW_ID,
        workflow_step_id: "step-2",
        entry_options: {
          instructions: "  inspect the destination  ",
          reset_context: true,
        },
      },
      { baseUrl: API_BASE_URL },
    );

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks/task-1/move-preview`);
    expect(init).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        workflow_id: WORKFLOW_ID,
        workflow_step_id: "step-2",
        entry_options: {
          reset_context: true,
          instructions: "inspect the destination",
        },
      }),
    });
    expect((init as RequestInit).cache).toBe("no-store");
  });
});

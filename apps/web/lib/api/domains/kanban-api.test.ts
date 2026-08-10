import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  attachTaskWorkspaceSources,
  detachTask,
  listTasksByWorkspace,
  updateTaskPortForwarding,
} from "./kanban-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

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

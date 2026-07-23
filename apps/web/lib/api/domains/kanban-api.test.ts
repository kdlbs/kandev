import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { attachTaskWorkspaceSources, detachTask } from "./kanban-api";

const fetchSpy = vi.fn<typeof fetch>();

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

    await detachTask("child-1", { baseUrl: "http://api.test" });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe("http://api.test/api/v1/tasks/child-1/detach");
    expect(init?.method).toBe("POST");
    expect(init?.body).toBeUndefined();
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
        { baseUrl: "http://api.test" },
      ),
    ).resolves.toMatchObject({ task_id: "task-1", workspace_path: "/workspace/task-1" });

    expect(fetchSpy).toHaveBeenCalledWith("http://api.test/api/v1/tasks/task-1/workspace-sources", {
      method: "POST",
      body: JSON.stringify({
        sources: [{ kind: "folder", local_path: "/docs", display_name: "docs" }],
      }),
      cache: undefined,
      headers: { "Content-Type": "application/json" },
    });
  });

  it("preserves normalized API errors for retry UI", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "task has an active turn" }), {
        status: 409,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(
      attachTaskWorkspaceSources("task-1", { sources: [] }, { baseUrl: "http://api.test" }),
    ).rejects.toMatchObject({ name: "ApiError", status: 409, message: "task has an active turn" });
  });
});

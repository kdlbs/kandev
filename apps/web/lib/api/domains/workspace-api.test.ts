import { afterEach, describe, expect, it, vi } from "vitest";
import { initializeLocalRepository, restartConfigChat } from "./workspace-api";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
});

describe("restartConfigChat", () => {
  it("posts the exact confirmed pair and native deletion ticket once", async () => {
    const fetchSpy = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ task_id: "new-task", session_id: "new-session" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    );
    global.fetch = fetchSpy as typeof global.fetch;
    await restartConfigChat("ws-1", { task_id: "old-task", session_id: "old-session" }, "ticket-1");
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    expect(fetchSpy).toHaveBeenCalledWith(
      "http://localhost:3000/api/v1/workspaces/ws-1/config-chat/restart",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ task_id: "old-task", session_id: "old-session" }),
      }),
    );
    expect(
      new Headers(fetchSpy.mock.calls[0][1]?.headers).get("X-Kandev-Task-Delete-Confirmation"),
    ).toBe("ticket-1");
  });

  it("preserves stage and replacement identity on failure without retrying", async () => {
    const failure = {
      code: "config_chat_restart_start_failed",
      stage: "start",
      old_deleted: true,
      replacement: { task_id: "new-task", session_id: "new-session" },
    };
    const fetchSpy = vi.fn(
      async () =>
        new Response(JSON.stringify(failure), {
          status: 500,
          headers: { "Content-Type": "application/json" },
        }),
    );
    global.fetch = fetchSpy as typeof global.fetch;
    await expect(
      restartConfigChat("ws-1", { task_id: "old-task", session_id: "old-session" }, "ticket-1"),
    ).rejects.toMatchObject({ body: failure });
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });
});

describe("initializeLocalRepository", () => {
  it("posts the backend's snake-case initialization payload", async () => {
    const fetchSpy = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            id: "repo-1",
            workspace_id: "ws-1",
            name: "alpha",
            source_type: "local",
            local_path: "/work/alpha",
            default_branch: "main",
          }),
          { status: 201, headers: { "Content-Type": "application/json" } },
        ),
    );
    global.fetch = fetchSpy as typeof global.fetch;

    const repository = await initializeLocalRepository("ws-1", {
      name: "alpha",
      parentPath: "/work",
    });

    expect(fetchSpy).toHaveBeenCalledWith(
      "http://localhost:3000/api/v1/workspaces/ws-1/repositories/initialize-local",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ name: "alpha", parent_path: "/work" }),
      }),
    );
    expect(repository).toMatchObject({ id: "repo-1", default_branch: "main" });
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { initializeLocalRepository } from "./workspace-api";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
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

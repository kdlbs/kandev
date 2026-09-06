import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { getTaskDependencyCycle, replaceTaskDependencies } from "./task-dependencies-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("replaceTaskDependencies", () => {
  it("replaces the complete predecessor set with a PUT request", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          task_id: "task-1",
          blocked: true,
          blocked_reason: "pending",
          depends_on: [{ id: "task-2", title: "Predecessor" }],
          blocks: [],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await expect(
      replaceTaskDependencies("task-1", ["task-2"], { baseUrl: API_BASE_URL }),
    ).resolves.toMatchObject({ task_id: "task-1", blocked: true });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks/task-1/dependencies`);
    expect(init).toMatchObject({
      method: "PUT",
      body: JSON.stringify({ depends_on_task_ids: ["task-2"] }),
    });
    expect(new Headers(init?.headers).get("Content-Type")).toBe("application/json");
  });

  it("does not let caller request options override the replacement method or body", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ task_id: "task-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await replaceTaskDependencies("task-1", ["task-2"], {
      baseUrl: API_BASE_URL,
      init: { method: "POST", body: "caller-controlled body" },
    });

    const [, init] = fetchSpy.mock.calls[0];
    expect(init?.method).toBe("PUT");
    expect(init?.body).toBe(JSON.stringify({ depends_on_task_ids: ["task-2"] }));
  });

  it("keeps the structured cycle path available to the edit dialog", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: "would create a dependency cycle",
          cycle: ["task-1", "task-2", "task-1"],
        }),
        { status: 409, headers: { "Content-Type": "application/json" } },
      ),
    );

    let error: unknown;
    try {
      await replaceTaskDependencies("task-1", ["task-2"], { baseUrl: API_BASE_URL });
    } catch (caught) {
      error = caught;
    }

    expect(error).toBeInstanceOf(ApiError);
    expect(getTaskDependencyCycle(error)).toEqual(["task-1", "task-2", "task-1"]);
  });
});

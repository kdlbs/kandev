import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { updateTaskSessionAutoProvisioning } from "./kanban-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("updateTaskSessionAutoProvisioning", () => {
  it("patches only the task-scoped suppression marker", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "task-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await updateTaskSessionAutoProvisioning("task-1", true, { baseUrl: API_BASE_URL });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks/task-1/session-auto-provisioning`);
    expect(init).toMatchObject({ method: "PATCH", body: JSON.stringify({ suppressed: true }) });
  });
});

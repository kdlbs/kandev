import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { detachTask } from "./kanban-api";

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

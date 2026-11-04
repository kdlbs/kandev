import { afterEach, describe, expect, it, vi } from "vitest";
import { removeLabel } from "./office-extended-api";

afterEach(() => vi.unstubAllGlobals());

// @covers AC-TASKS-LABELS-002.1, AC-TASKS-LABELS-002.3, AC-TASKS-LABELS-002.6
describe("removeLabel exact-name transport", () => {
  it.each(["area/backend", "a/b/c", "area%2Fbackend", "a+b", "a b", "100%", " edge "])(
    "sends %s once in the query without changing identity",
    async (name) => {
      const fetchSpy = vi.fn().mockResolvedValue(Response.json({ ok: true }));
      vi.stubGlobal("fetch", fetchSpy);

      await removeLabel("workspace", "task", name, { baseUrl: "https://labels.test" });

      const [url, init] = fetchSpy.mock.calls[0] as [string, RequestInit];
      const request = new URL(url);
      expect(request.pathname).toBe("/api/v1/office/workspaces/workspace/tasks/task/labels");
      expect(request.searchParams.getAll("name")).toEqual([name]);
      expect(init.method).toBe("DELETE");
      expect(init.body).toBeUndefined();
    },
  );

  it("preserves caller options and propagates a rejected removal", async () => {
    const fetchSpy = vi
      .fn()
      .mockResolvedValue(Response.json({ error: "removal refused" }, { status: 403 }));
    vi.stubGlobal("fetch", fetchSpy);
    const controller = new AbortController();

    await expect(
      removeLabel("workspace", "task", "area/backend", {
        baseUrl: "https://labels.test",
        cache: "no-store",
        init: { signal: controller.signal, headers: { "X-Test": "preserved" } },
      }),
    ).rejects.toMatchObject({ status: 403, message: "removal refused" });

    const [, init] = fetchSpy.mock.calls[0] as [string, RequestInit];
    expect(init).toMatchObject({ method: "DELETE", cache: "no-store", signal: controller.signal });
    expect(new Headers(init.headers).get("X-Test")).toBe("preserved");
  });
});

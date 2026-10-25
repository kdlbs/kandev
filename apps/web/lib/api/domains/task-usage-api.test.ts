import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));

import { fetchTaskUsageTotals } from "./task-usage-api";

afterEach(() => vi.unstubAllGlobals());

describe("fetchTaskUsageTotals", () => {
  it("reads task-wide usage from the authenticated task route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ scope: "task", scope_id: "task/1", tokens_total: 6 }), {
        status: 200,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchTaskUsageTotals("task/1")).resolves.toMatchObject({
      scope: "task",
      tokens_total: 6,
    });
    expect(fetchMock.mock.calls[0][0]).toBe("http://api.test/api/v1/tasks/task%2F1/usage");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ credentials: "include" });
  });

  it("reads session-scoped usage without exposing another task's totals", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ scope: "session" }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await fetchTaskUsageTotals("task-1", "session-2");
    expect(fetchMock.mock.calls[0][0]).toBe(
      "http://api.test/api/v1/tasks/task-1/sessions/session-2/usage",
    );
  });
});

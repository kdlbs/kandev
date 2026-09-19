import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { retryTaskCIAutoMerge } from "./github-api";

const fetchSpy = vi.fn<(...args: Parameters<typeof fetch>) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("GitHub automatic merge retry", () => {
  it("requests one retry for the exact linked PR", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ accepted: true }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await retryTaskCIAutoMerge("task-1", "repo-1", 42);

    const call = fetchSpy.mock.calls.at(-1);
    expect(String(call?.[0])).toBe(
      "http://api.test/api/v1/github/tasks/task-1/ci-automation/retry-merge",
    );
    expect(call?.[1]?.method).toBe("POST");
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({
      repository_id: "repo-1",
      pr_number: 42,
    });
  });
});

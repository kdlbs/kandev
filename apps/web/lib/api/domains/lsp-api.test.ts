import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { getTaskLsp, restartTaskLsp, setTaskLspPolicy, startTaskLsp, stopTaskLsp } from "./lsp-api";

const fetchSpy = vi.fn<(...args: Parameters<typeof fetch>) => Promise<Response>>();

function response(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  fetchSpy.mockReset();
  fetchSpy.mockResolvedValue(response({ task_id: "task/1", languages: [], capacity: {} }));
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("task LSP API", () => {
  it("reads the task-scoped snapshot", async () => {
    await getTaskLsp("task/1");
    expect(fetchSpy.mock.calls[0]?.[0]).toBe("http://api.test/api/v1/tasks/task%2F1/lsp");
    expect(fetchSpy.mock.calls[0]?.[1]?.method).toBeUndefined();
  });

  it("sends only policy in the policy request", async () => {
    await setTaskLspPolicy("task/1", "type/script", "keep_warm");
    const [url, init] = fetchSpy.mock.calls[0] ?? [];
    expect(url).toBe("http://api.test/api/v1/tasks/task%2F1/lsp/type%2Fscript/policy");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ policy: "keep_warm" });
  });

  it.each([
    ["start", startTaskLsp],
    ["stop", stopTaskLsp],
    ["restart", restartTaskLsp],
  ] as const)("sends an empty %s request without identity or origin", async (action, invoke) => {
    await invoke("task/1", "type/script");
    const [url, init] = fetchSpy.mock.calls[0] ?? [];
    expect(url).toBe(`http://api.test/api/v1/tasks/task%2F1/lsp/type%2Fscript/${action}`);
    expect(init?.method).toBe("POST");
    expect(init?.body).toBeUndefined();
  });
});

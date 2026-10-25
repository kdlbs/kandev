import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readRelay } from "./coordinator-relay-api";

const fetchSpy = vi.fn<typeof fetch>();
const OPTS = { baseUrl: "http://api.test" };

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});
afterEach(() => vi.unstubAllGlobals());

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("readRelay", () => {
  it("reads the relay route with encoded ids and returns the body", async () => {
    const relay = { task_id: "t 1", session_id: "s1", clarification: null, permission: null };
    fetchSpy.mockResolvedValue(json(relay));
    const result = await readRelay("ws-1", "co-1", "t 1", OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(
      "http://api.test/api/v1/workspaces/ws-1/coordinators/co-1/relay/t%201",
    );
    expect(result).toEqual({ kind: "ok", relay });
  });

  it.each([403, 404])("reports %i as refused", async (status) => {
    fetchSpy.mockResolvedValue(json({ error: "x" }, status));
    expect(await readRelay("ws-1", "co-1", "t1", OPTS)).toEqual({ kind: "refused" });
  });

  it.each([400, 500])("reports %i as failed", async (status) => {
    fetchSpy.mockResolvedValue(json({ error: "x" }, status));
    expect(await readRelay("ws-1", "co-1", "t1", OPTS)).toEqual({ kind: "failed" });
  });

  it("reports a network error as failed", async () => {
    fetchSpy.mockRejectedValue(new TypeError("offline"));
    expect(await readRelay("ws-1", "co-1", "t1", OPTS)).toEqual({ kind: "failed" });
  });
});

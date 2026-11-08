import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  checkMergeNow,
  getDependencies,
  getMergeProposal,
  getTaskChanges,
  mergeErrorCode,
  putDependencies,
  refreshMergeProtection,
  releaseMerge,
} from "./coordinator-merge-api";

const fetchSpy = vi.fn<typeof fetch>();
const OPTS = { baseUrl: "http://api.test" };
const BASE = "http://api.test/api/v1/workspaces/ws-1/coordinators/co-1";

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

function lastCall() {
  const [url, init] = fetchSpy.mock.calls[fetchSpy.mock.calls.length - 1];
  return { url: String(url), init };
}

describe("dependencies", () => {
  it("reads the set with either revision key", async () => {
    fetchSpy.mockResolvedValueOnce(json({ refs: [{ task: "t2" }], revision: 3 }));
    expect(await getDependencies("ws-1", "co-1", "t1", OPTS)).toEqual({
      refs: [{ task: "t2" }],
      revision: 3,
    });
    expect(lastCall().url).toBe(`${BASE}/tasks/t1/dependencies`);

    fetchSpy.mockResolvedValueOnce(json({ refs: null, dependency_revision: 5 }));
    expect(await getDependencies("ws-1", "co-1", "t1", OPTS)).toEqual({ refs: [], revision: 5 });
  });

  it("saves under dependency_revision", async () => {
    fetchSpy.mockResolvedValueOnce(json({ refs: [{ change: "github|h|r|1" }], revision: 4 }));
    const set = await putDependencies("ws-1", "co-1", "t1", 3, [{ change: "github|h|r|1" }]);
    expect(set.revision).toBe(4);
    const { url, init } = lastCall();
    expect(url.endsWith("/coordinators/co-1/tasks/t1/dependencies")).toBe(true);
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({
      dependency_revision: 3,
      refs: [{ change: "github|h|r|1" }],
    });
  });
});

describe("merge proposal routes", () => {
  it("reads the detail", async () => {
    fetchSpy.mockResolvedValueOnce(json({ id: "p1" }));
    await getMergeProposal("ws-1", "co-1", "p1", OPTS);
    expect(lastCall().url).toBe(`${BASE}/proposals/p1`);
  });

  it("posts check and release", async () => {
    fetchSpy.mockImplementation(async () => json({ id: "p1", status: "uncertain" }));
    await checkMergeNow("ws-1", "co-1", "p1", OPTS);
    expect(lastCall().url).toBe(`${BASE}/proposals/p1/merge/check`);
    expect(lastCall().init?.method).toBe("POST");
    await releaseMerge("ws-1", "co-1", "p1", OPTS);
    expect(lastCall().url).toBe(`${BASE}/proposals/p1/merge/release`);
  });

  it("refreshes protection", async () => {
    fetchSpy.mockResolvedValueOnce(json({ protection: [] }));
    await refreshMergeProtection("ws-1", "co-1", OPTS);
    expect(lastCall().url).toBe(`${BASE}/merge/protection/refresh`);
    expect(lastCall().init?.method).toBe("POST");
  });
});

describe("getTaskChanges", () => {
  it("adds retry only when asked", async () => {
    fetchSpy.mockImplementation(async () => json({ changes: [] }));
    await getTaskChanges("ws-1", "co-1", "t1", false, OPTS);
    expect(lastCall().url).toBe(`${BASE}/tasks/t1/changes`);
    await getTaskChanges("ws-1", "co-1", "t1", true, OPTS);
    expect(lastCall().url).toBe(`${BASE}/tasks/t1/changes?retry=1`);
  });

  it("treats a null list as empty and a 404 as none", async () => {
    fetchSpy.mockResolvedValueOnce(json({ changes: null }));
    expect(await getTaskChanges("ws-1", "co-1", "t1", false, OPTS)).toEqual([]);
    fetchSpy.mockResolvedValueOnce(json({ error: "not found" }, 404));
    expect(await getTaskChanges("ws-1", "co-1", "t1", false, OPTS)).toBeNull();
  });

  it("rethrows other failures", async () => {
    fetchSpy.mockResolvedValueOnce(json({ error: "boom" }, 500));
    await expect(getTaskChanges("ws-1", "co-1", "t1", false, OPTS)).rejects.toBeInstanceOf(
      ApiError,
    );
  });
});

describe("mergeErrorCode", () => {
  it("reads code or error from a 400 or 409 body", () => {
    expect(mergeErrorCode(new ApiError("x", 409, { code: "principal_valid" }))).toBe(
      "principal_valid",
    );
    expect(mergeErrorCode(new ApiError("x", 400, { error: "dependency_self" }))).toBe(
      "dependency_self",
    );
  });

  it("is null for other statuses and non-API errors", () => {
    expect(mergeErrorCode(new ApiError("x", 500, { code: "a" }))).toBeNull();
    expect(mergeErrorCode(new Error("x"))).toBeNull();
    expect(mergeErrorCode(new ApiError("x", 409, null))).toBeNull();
  });
});

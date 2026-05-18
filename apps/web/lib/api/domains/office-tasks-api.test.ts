import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Pin the backend config so URL assertions are deterministic.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { listTasks, type ListTasksParams } from "./office-tasks-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<[FetchInput, FetchInit?], Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
  fetchSpy.mockResolvedValue(
    new Response(JSON.stringify({ tasks: [] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function lastUrl(): string {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return String(call[0]);
}

const BASE = "http://api.test/api/v1/office/workspaces/ws-1/tasks";

describe("listTasks — query string building", () => {
  it("emits no query string when no params are passed", async () => {
    await listTasks("ws-1");
    expect(lastUrl()).toBe(BASE);
  });

  it("emits no query string when params object is empty", async () => {
    await listTasks("ws-1", {} as ListTasksParams);
    expect(lastUrl()).toBe(BASE);
  });

  it("repeats status as multiple query params", async () => {
    await listTasks("ws-1", { status: ["TODO", "IN_PROGRESS", "REVIEW"] });
    const url = new URL(lastUrl());
    expect(url.searchParams.getAll("status")).toEqual(["TODO", "IN_PROGRESS", "REVIEW"]);
  });

  it("repeats priority as multiple query params", async () => {
    await listTasks("ws-1", { priority: ["high", "low"] });
    const url = new URL(lastUrl());
    expect(url.searchParams.getAll("priority")).toEqual(["high", "low"]);
  });

  it("includes assignee, project, sort, order, and limit when set", async () => {
    await listTasks("ws-1", {
      assignee: "agent-1",
      project: "proj-1",
      sort: "updated_at",
      order: "desc",
      limit: 50,
    });
    const url = new URL(lastUrl());
    expect(url.searchParams.get("assignee")).toBe("agent-1");
    expect(url.searchParams.get("project")).toBe("proj-1");
    expect(url.searchParams.get("sort")).toBe("updated_at");
    expect(url.searchParams.get("order")).toBe("desc");
    expect(url.searchParams.get("limit")).toBe("50");
  });

  it("includes both cursor and cursor_id for keyset pagination", async () => {
    await listTasks("ws-1", { cursor: "2026-01-01T00:00:00Z", cursor_id: "task-42" });
    const url = new URL(lastUrl());
    expect(url.searchParams.get("cursor")).toBe("2026-01-01T00:00:00Z");
    expect(url.searchParams.get("cursor_id")).toBe("task-42");
  });

  it("excludes undefined fields from the query string", async () => {
    await listTasks("ws-1", { sort: "created_at" });
    const url = new URL(lastUrl());
    expect(url.searchParams.has("assignee")).toBe(false);
    expect(url.searchParams.has("project")).toBe(false);
    expect(url.searchParams.has("limit")).toBe(false);
    expect(url.searchParams.has("cursor")).toBe(false);
    expect(url.searchParams.get("sort")).toBe("created_at");
  });

  it("treats a second arg with ApiRequestOptions keys as legacy options, not params", async () => {
    await listTasks("ws-1", { cache: "no-store" });
    expect(lastUrl()).toBe(BASE);
    const init = fetchSpy.mock.calls.at(-1)?.[1];
    expect(init?.cache).toBe("no-store");
  });

  it("treats a second arg without ApiRequestOptions keys as params", async () => {
    await listTasks("ws-1", { status: ["DONE"] });
    expect(lastUrl()).toContain("status=DONE");
  });
});

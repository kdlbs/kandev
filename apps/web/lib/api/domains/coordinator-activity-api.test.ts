import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  getUndoConflict,
  isActivityClass,
  listActivity,
  undoActivity,
} from "./coordinator-activity-api";

const fetchSpy = vi.fn<typeof fetch>();
const OPTS = { baseUrl: "http://api.test" };
const BASE = "http://api.test/api/v1/workspaces/ws-1/coordinators/co-1/activity";

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

describe("listActivity", () => {
  it("sends no query for the first All page and never a limit", async () => {
    fetchSpy.mockResolvedValue(json({ rows: [], next_cursor: null }));
    await listActivity("ws-1", "co-1", {}, OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(BASE);
  });

  it("sends class and cursor", async () => {
    fetchSpy.mockResolvedValue(json({ rows: [], next_cursor: null }));
    await listActivity("ws-1", "co-1", { class: "move", before: "abc" }, OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${BASE}?class=move&before=abc`);
  });
});

describe("undoActivity", () => {
  it("posts to the row's undo route", async () => {
    fetchSpy.mockResolvedValue(json({ id: "r1" }));
    await undoActivity("ws-1", "co-1", "r1", OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${BASE}/r1/undo`);
    expect(fetchSpy.mock.calls[0][1]?.method).toBe("POST");
  });
});

describe("getUndoConflict", () => {
  it("reads a known code with its reason", () => {
    const err = new ApiError("x", 409, { code: "undo_conflict", reason: "step_full" });
    expect(getUndoConflict(err)).toEqual({ code: "undo_conflict", reason: "step_full" });
  });

  it("reads a known code without a reason", () => {
    const err = new ApiError("x", 409, { code: "not_undoable" });
    expect(getUndoConflict(err)).toEqual({ code: "not_undoable" });
  });

  it("returns null for an unknown code, a non-409 and a non-ApiError", () => {
    expect(getUndoConflict(new ApiError("x", 409, { code: "new_code" }))).toBeNull();
    expect(getUndoConflict(new ApiError("x", 500, { code: "undo_conflict" }))).toBeNull();
    expect(getUndoConflict(new Error("x"))).toBeNull();
    expect(getUndoConflict(new ApiError("x", 409, null))).toBeNull();
  });
});

describe("isActivityClass", () => {
  it("accepts the seven classes and unknown, rejects anything else", () => {
    expect(isActivityClass("move")).toBe(true);
    expect(isActivityClass("unknown")).toBe(true);
    expect(isActivityClass("improvement")).toBe(true);
    expect(isActivityClass("bogus")).toBe(false);
    expect(isActivityClass(null)).toBe(false);
  });
});

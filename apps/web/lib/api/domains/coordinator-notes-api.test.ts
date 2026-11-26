import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  type NoteDraft,
  createNote,
  deleteNote,
  editNote,
  getNote,
  getNoteReferences,
  listNotes,
  noteFailure,
  revertNote,
} from "./coordinator-notes-api";

const fetchSpy = vi.fn<typeof fetch>();
const OPTS = { baseUrl: "http://api.test" };
const BASE = "http://api.test/api/v1/workspaces/ws-1/coordinators/co-1/notes";
const DRAFT: NoteDraft = { type: "project", text: "x", citations: [] };

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
  const [url, init] = fetchSpy.mock.calls[0];
  return { url, init: init as RequestInit, body: JSON.parse(String((init as RequestInit).body)) };
}

describe("coordinator notes api", () => {
  it("lists notes, with the include-deleted query only when asked", async () => {
    fetchSpy.mockImplementation(async () => json({ notes: [{ id: "n1" }] }));
    expect(await listNotes("ws-1", "co-1", false, OPTS)).toEqual([{ id: "n1" }]);
    expect(fetchSpy.mock.calls[0][0]).toBe(BASE);
    await listNotes("ws-1", "co-1", true, OPTS);
    expect(fetchSpy.mock.calls[1][0]).toBe(`${BASE}?include_deleted=true`);
  });

  it("reads a note and its reference history", async () => {
    fetchSpy.mockResolvedValue(json({ id: "n 1", versions: [] }));
    await getNote("ws-1", "co-1", "n 1", OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${BASE}/n%201`);
    fetchSpy.mockResolvedValue(
      json({ references: [{ proposal_id: "p", status: "x", note_version: 1 }] }),
    );
    expect(await getNoteReferences("ws-1", "co-1", "n1", OPTS)).toHaveLength(1);
    expect(fetchSpy.mock.calls[1][0]).toBe(`${BASE}/n1/references`);
  });

  it("sends create, edit, revert and delete with the version they act on", async () => {
    fetchSpy.mockImplementation(async () => json({ id: "n1" }));
    await createNote("ws-1", "co-1", DRAFT, OPTS);
    expect(lastCall().init.method).toBe("POST");
    expect(lastCall().body).toEqual(DRAFT);
    fetchSpy.mockClear();
    await editNote("ws-1", "co-1", "n1", { ...DRAFT, version: 3 }, OPTS);
    expect(lastCall()).toMatchObject({ url: `${BASE}/n1`, body: { ...DRAFT, version: 3 } });
    expect(lastCall().init.method).toBe("PUT");
    fetchSpy.mockClear();
    await revertNote("ws-1", "co-1", "n1", { target_version: 1, version: 3 }, OPTS);
    expect(lastCall()).toMatchObject({
      url: `${BASE}/n1/revert`,
      body: { target_version: 1, version: 3 },
    });
    fetchSpy.mockClear();
    await deleteNote("ws-1", "co-1", "n1", 3, OPTS);
    expect(lastCall()).toMatchObject({ url: `${BASE}/n1/delete`, body: { version: 3 } });
  });

  it("reads the reason set from a rejected request", async () => {
    fetchSpy.mockResolvedValue(
      json({ error: "conflict", code: "conflict", reason: "expired" }, 409),
    );
    const err = await editNote("ws-1", "co-1", "n1", { ...DRAFT, version: 1 }, OPTS).catch(
      (e) => e,
    );
    expect(err).toBeInstanceOf(ApiError);
    expect(noteFailure(err)).toEqual({ code: "conflict", reason: "expired" });
    const field = new ApiError("bad", 400, {
      code: "invalid_citation",
      field: "citations",
      index: 2,
    });
    expect(noteFailure(field)).toMatchObject({
      code: "invalid_citation",
      field: "citations",
      index: 2,
    });
    expect(noteFailure(new Error("network"))).toEqual({ code: "unknown" });
    expect(noteFailure(new ApiError("x", 500, null))).toEqual({ code: "unknown" });
  });
});

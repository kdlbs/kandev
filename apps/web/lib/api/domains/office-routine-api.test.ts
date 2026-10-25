import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getRoutine, listRoutines } from "./office-routine-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";
const WORKSPACE_ID = "workspace-1";
const ROUTINE_ID = "routine-1";
const MALFORMED_ROUTINE_ERROR = 'office API: malformed "routine" response';

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

// AC-003.5: unwrapEnvelopeList takes the missing/null/non-array directions to
// an empty list rather than throwing, so a malformed collection envelope
// degrades to "nothing to show" instead of crashing the page.
describe("listRoutines envelope unwrap (AC-003.5)", () => {
  it("returns an empty list when the envelope key is missing", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}));
    await expect(listRoutines(WORKSPACE_ID, { baseUrl: API_BASE_URL })).resolves.toEqual({
      routines: [],
    });
  });

  it("returns an empty list when the envelope key is null", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routines: null }));
    await expect(listRoutines(WORKSPACE_ID, { baseUrl: API_BASE_URL })).resolves.toEqual({
      routines: [],
    });
  });

  it("returns an empty list when the envelope key is not an array", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routines: "not-an-array" }));
    await expect(listRoutines(WORKSPACE_ID, { baseUrl: API_BASE_URL })).resolves.toEqual({
      routines: [],
    });
  });

  it("returns an empty list when the response body itself is not an object", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(null));
    await expect(listRoutines(WORKSPACE_ID, { baseUrl: API_BASE_URL })).resolves.toEqual({
      routines: [],
    });
  });

  it("passes a well-formed list through to normalization", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routines: [{ id: "r1", name: "Sweep" }] }));
    const result = await listRoutines(WORKSPACE_ID, { baseUrl: API_BASE_URL });
    expect(result.routines).toHaveLength(1);
    expect(result.routines[0].id).toBe("r1");
  });
});

// AC-003.15: unwrapEnvelopeResource takes the missing/null/non-object/no-id
// directions to a thrown error instead of returning a value the caller would
// silently skip past — this is the exact silent-skip AC-003.15 exists to
// prevent.
describe("getRoutine envelope unwrap (AC-003.15)", () => {
  it("throws when the envelope key is missing", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}));
    await expect(getRoutine(ROUTINE_ID, { baseUrl: API_BASE_URL })).rejects.toThrow(
      MALFORMED_ROUTINE_ERROR,
    );
  });

  it("throws when the envelope key is null", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routine: null }));
    await expect(getRoutine(ROUTINE_ID, { baseUrl: API_BASE_URL })).rejects.toThrow(
      MALFORMED_ROUTINE_ERROR,
    );
  });

  it("throws when the envelope key is not an object", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routine: "not-an-object" }));
    await expect(getRoutine(ROUTINE_ID, { baseUrl: API_BASE_URL })).rejects.toThrow(
      MALFORMED_ROUTINE_ERROR,
    );
  });

  it("throws when the envelope key is an array", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routine: [] }));
    await expect(getRoutine(ROUTINE_ID, { baseUrl: API_BASE_URL })).rejects.toThrow(
      MALFORMED_ROUTINE_ERROR,
    );
  });

  it("throws when the resource has no id field", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routine: { name: "Sweep" } }));
    await expect(getRoutine(ROUTINE_ID, { baseUrl: API_BASE_URL })).rejects.toThrow(
      MALFORMED_ROUTINE_ERROR,
    );
  });

  it("throws when the resource's id is an empty string", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routine: { id: "" } }));
    await expect(getRoutine(ROUTINE_ID, { baseUrl: API_BASE_URL })).rejects.toThrow(
      MALFORMED_ROUTINE_ERROR,
    );
  });

  it("throws when the resource's id is not a string", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routine: { id: 123 } }));
    await expect(getRoutine(ROUTINE_ID, { baseUrl: API_BASE_URL })).rejects.toThrow(
      MALFORMED_ROUTINE_ERROR,
    );
  });

  it("returns the normalized resource when it has a non-empty string id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ routine: { id: "r1", name: "Sweep" } }));
    const result = await getRoutine(ROUTINE_ID, { baseUrl: API_BASE_URL });
    expect(result.id).toBe("r1");
    expect(result.name).toBe("Sweep");
  });
});

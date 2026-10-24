import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  getDream,
  getLearning,
  getOutcomeMeasures,
  listDreams,
  putDreamRating,
  putShadowDream,
} from "./coordinator-learning-api";

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

describe("coordinator learning api", () => {
  it("reads the learning view", async () => {
    const view = { shadow_dream: true, health: { state: "fresh" } };
    fetchSpy.mockResolvedValue(json(view));
    expect(await getLearning("ws-1", "co-1", OPTS)).toEqual(view);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${BASE}/learning`);
  });

  it("puts the switch as a JSON boolean", async () => {
    fetchSpy.mockResolvedValue(json({ shadow_dream: false, health: { state: "off" } }));
    await putShadowDream("ws-1", "co-1", false, OPTS);
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${BASE}/learning`);
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ shadow_dream: false });
  });

  it("reads measures for a window", async () => {
    fetchSpy.mockResolvedValue(json({ days: 7 }));
    await getOutcomeMeasures("ws-1", "co-1", 7, OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${BASE}/measures?days=7`);
  });

  it("pages the reports with an encoded cursor", async () => {
    fetchSpy.mockImplementation(() => Promise.resolve(json({ dreams: [] })));
    await listDreams("ws-1", "co-1", undefined, OPTS);
    await listDreams("ws-1", "co-1", "2026-09-01T00:00:00Z,d 1", OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${BASE}/dreams`);
    expect(fetchSpy.mock.calls[1][0]).toBe(
      `${BASE}/dreams?before=2026-09-01T00%3A00%3A00Z%2Cd%201`,
    );
  });

  it("reads one report by encoded id", async () => {
    fetchSpy.mockResolvedValue(json({ id: "d 1" }));
    await getDream("ws-1", "co-1", "d 1", OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${BASE}/dreams/d%201`);
  });

  it("puts a rating and accepts the empty answer", async () => {
    fetchSpy.mockResolvedValue(new Response(null, { status: 204 }));
    const target = { workspaceId: "ws-1", coordinatorId: "co-1", dreamId: "d1", itemId: "i1" };
    await putDreamRating(target, "harmful", OPTS);
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${BASE}/dreams/d1/items/i1/rating`);
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ rating: "harmful" });
  });
});

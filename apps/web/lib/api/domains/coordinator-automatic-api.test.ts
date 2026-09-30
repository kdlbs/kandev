import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getClassEligibility, recordClassReview } from "./coordinator-automatic-api";

const fetchSpy = vi.fn<typeof fetch>();
const OPTS = { baseUrl: "http://api.test" };
const BASE = "http://api.test/api/v1/workspaces/ws-1/coordinators/coord-1/classes/create_task";

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

describe("automatic class API", () => {
  it("reads eligibility from the class route", async () => {
    const body = {
      eligible: false,
      conditions: [],
      setting: "requires_approval",
      changed_by: "",
      changed_at: null,
    };
    fetchSpy.mockResolvedValueOnce(json(body));
    await expect(getClassEligibility("ws-1", "coord-1", "create_task", OPTS)).resolves.toEqual(
      body,
    );
    expect(fetchSpy.mock.calls[0][0]).toBe(`${BASE}/eligibility`);
  });

  it("records a review with a POST that carries no client fields", async () => {
    fetchSpy.mockResolvedValueOnce(json({ id: "r1" }, 201));
    await recordClassReview("ws-1", "coord-1", "create_task", OPTS);
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${BASE}/reviews`);
    expect(init?.method).toBe("POST");
    expect(init?.body).toBeUndefined();
  });

  it("rejects when the review is refused", async () => {
    fetchSpy.mockResolvedValueOnce(json({ error: "forbidden" }, 403));
    await expect(recordClassReview("ws-1", "coord-1", "create_task", OPTS)).rejects.toBeTruthy();
  });
});

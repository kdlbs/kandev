import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { changeTaskManagementClaim, getTaskManagementClaim } from "./task-management-claims-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("task management claim API", () => {
  it("reads the claim and its audit history", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ claim: null, history: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(getTaskManagementClaim("task/1", { baseUrl: API_BASE_URL })).resolves.toEqual({
      claim: null,
      history: [],
    });

    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${API_BASE_URL}/api/v1/tasks/task%2F1/management-claim`,
    );
    expect(fetchSpy.mock.calls[0][1]?.method).toBeUndefined();
  });

  it("posts version-bound human transfer and release commands", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ task_id: "task-1", owner_kind: "human" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await changeTaskManagementClaim(
      "task-1",
      {
        action: "transfer",
        expected_task_resource_version: "task-rv-4",
        expected_claim_resource_version: "claim-rv-2",
        reason: "Take over the unavailable manager",
      },
      { baseUrl: API_BASE_URL },
    );

    expect(fetchSpy.mock.calls[0][0]).toBe(`${API_BASE_URL}/api/v1/tasks/task-1/management-claim`);
    expect(fetchSpy.mock.calls[0][1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        action: "transfer",
        expected_task_resource_version: "task-rv-4",
        expected_claim_resource_version: "claim-rv-2",
        reason: "Take over the unavailable manager",
      }),
    });
  });
});

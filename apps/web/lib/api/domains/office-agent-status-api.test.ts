import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { updateAgentStatus } from "./office-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";
const AGENT_ID = "agent-1";

function agentResponse(overrides: Record<string, unknown> = {}) {
  return {
    agent: {
      id: AGENT_ID,
      workspace_id: "ws-1",
      name: "Worker",
      role: "worker",
      status: "idle",
      ...overrides,
    },
  };
}

beforeEach(() => {
  fetchSpy.mockReset();
  fetchSpy.mockResolvedValue(
    new Response(JSON.stringify(agentResponse()), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("updateAgentStatus", () => {
  it("issues exactly one PATCH to the status endpoint with only the status field", async () => {
    await updateAgentStatus(AGENT_ID, "idle");

    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const [input, init] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(`${API_BASE_URL}/api/v1/office/agents/${AGENT_ID}/status`);
    expect(init?.method).toBe("PATCH");
    expect(init?.body).toBe(JSON.stringify({ status: "idle" }));
  });

  it("uses the caller's target status, not a value derived from a prior response", async () => {
    await updateAgentStatus(AGENT_ID, "stopped");

    const [, init] = fetchSpy.mock.calls[0] ?? [];
    expect(init?.body).toBe(JSON.stringify({ status: "stopped" }));
  });

  it("includes the rendered status when the caller requests a guarded recovery", async () => {
    await updateAgentStatus(AGENT_ID, "idle", { expectedStatus: "paused" });

    const [, init] = fetchSpy.mock.calls[0] ?? [];
    expect(init?.body).toBe(JSON.stringify({ status: "idle", expected_status: "paused" }));
  });

  it("resolves to the response body's agent, normalized", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify(agentResponse({ status: "idle", pause_reason: undefined })), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const result = await updateAgentStatus(AGENT_ID, "idle");

    expect(result.status).toBe("idle");
    expect(result.pauseReason).toBe("");
  });

  it("rejects when the transition is refused, without swallowing the error", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "invalid status transition" }), {
        status: 400,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(updateAgentStatus(AGENT_ID, "idle")).rejects.toThrow("invalid status transition");
  });
});

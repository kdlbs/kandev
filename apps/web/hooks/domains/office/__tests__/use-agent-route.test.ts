import { describe, it, expect, vi, beforeEach } from "vitest";
import { waitFor } from "@testing-library/react";
import { renderHookWithQueryClient } from "@/test-utils/render-with-query";
import { useAgentRoute } from "../use-agent-route";
import type { AgentRouteData, AgentRoutingOverrides } from "@/lib/state/slices/office/types";

const mocks = vi.hoisted(() => ({
  getAgentRoute: vi.fn<[string], Promise<AgentRouteData>>(),
  updateAgentRouting: vi.fn<[string, AgentRoutingOverrides], Promise<{ ok: boolean }>>(),
}));

vi.mock("@/lib/api/domains/office-routing-api", () => ({
  getAgentRoute: mocks.getAgentRoute,
  updateAgentRouting: mocks.updateAgentRouting,
}));

const baseData: AgentRouteData = {
  preview: {
    agent_id: "agent-1",
    agent_name: "Agent One",
    tier_source: "inherit",
    effective_tier: "frontier",
    fallback_chain: [],
    missing: [],
    degraded: false,
  },
  overrides: {},
};

describe("useAgentRoute", () => {
  beforeEach(() => {
    mocks.getAgentRoute.mockReset();
    mocks.updateAgentRouting.mockReset();
    mocks.getAgentRoute.mockResolvedValue(baseData);
    mocks.updateAgentRouting.mockResolvedValue({ ok: true });
  });

  it("returns undefined when agentId is null", () => {
    const { result } = renderHookWithQueryClient(() => useAgentRoute(null));
    expect(result.current.data).toBeUndefined();
    expect(result.current.isLoading).toBe(false);
  });

  it("fetches and returns route data", async () => {
    const { result } = renderHookWithQueryClient(() => useAgentRoute("agent-1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.data).toEqual(baseData);
  });

  it("updateOverrides calls the API and invalidates", async () => {
    const { result } = renderHookWithQueryClient(() => useAgentRoute("agent-1"));
    await waitFor(() => expect(result.current.data).toBeDefined());
    const overrides: AgentRoutingOverrides = { tier_source: "override", tier: "economy" };
    await result.current.updateOverrides(overrides);
    expect(mocks.updateAgentRouting).toHaveBeenCalledWith("agent-1", overrides);
  });

  it("error is set when fetch fails", async () => {
    mocks.getAgentRoute.mockRejectedValue(new Error("agent not found"));
    const { result } = renderHookWithQueryClient(() => useAgentRoute("bad-agent"));
    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.error).toBe("agent not found");
  });
});

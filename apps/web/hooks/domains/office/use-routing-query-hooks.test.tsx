import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentRouteData,
  AgentRoutePreview,
  ProviderHealth,
  RouteAttempt,
} from "@/lib/state/slices/office/types";
import { useAgentRoute } from "./use-agent-route";
import { useProviderHealth } from "./use-provider-health";
import { useRoutingPreview } from "./use-routing-preview";
import { useRunAttempts } from "./use-run-attempts";

const routingMocks = vi.hoisted(() => ({
  getAgentRoute: vi.fn(),
  getProviderHealth: vi.fn(),
  getRoutingPreview: vi.fn(),
  getWorkspaceRouting: vi.fn(),
}));

const runMocks = vi.hoisted(() => ({
  getRunAttempts: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-routing-api", () => ({
  getAgentRoute: routingMocks.getAgentRoute,
  getProviderHealth: routingMocks.getProviderHealth,
  getRoutingPreview: routingMocks.getRoutingPreview,
  getWorkspaceRouting: routingMocks.getWorkspaceRouting,
}));

vi.mock("@/lib/api/domains/office-runs-api", () => ({
  getRunAttempts: runMocks.getRunAttempts,
}));

describe("Office routing query hooks", () => {
  beforeEach(() => {
    routingMocks.getAgentRoute.mockReset();
    routingMocks.getProviderHealth.mockReset();
    routingMocks.getRoutingPreview.mockReset();
    routingMocks.getWorkspaceRouting.mockReset();
    runMocks.getRunAttempts.mockReset();
  });

  it("reads provider health from the query result", async () => {
    const health: ProviderHealth[] = [
      {
        provider_id: "claude-acp",
        scope: "provider",
        scope_value: "claude-acp",
        state: "healthy",
        backoff_step: 0,
      },
    ];
    routingMocks.getProviderHealth.mockResolvedValue({ health });

    const { result } = renderHook(() => useProviderHealth("ws-1"), {
      wrapper: createQueryWrapper(),
    });

    await waitFor(() => expect(result.current.health).toEqual(health));
  });

  it("reads routing preview agents from the query result", async () => {
    const agents: AgentRoutePreview[] = [
      {
        agent_id: "agent-1",
        agent_name: "Builder",
        tier_source: "inherit",
        effective_tier: "balanced",
        fallback_chain: [],
        missing: [],
        degraded: false,
      },
    ];
    routingMocks.getRoutingPreview.mockResolvedValue({ agents });

    const { result } = renderHook(() => useRoutingPreview("ws-1"), {
      wrapper: createQueryWrapper(),
    });

    await waitFor(() => expect(result.current.agents).toEqual(agents));
  });

  it("reads run attempts from the query result", async () => {
    const attempts: RouteAttempt[] = [
      {
        seq: 1,
        provider_id: "claude-acp",
        tier: "balanced",
        outcome: "launched",
        started_at: "2026-01-01T00:00:00Z",
      },
    ];
    runMocks.getRunAttempts.mockResolvedValue({ attempts });

    const { result } = renderHook(() => useRunAttempts("run-1"), {
      wrapper: createQueryWrapper(),
    });

    await waitFor(() => expect(result.current.attempts).toEqual(attempts));
  });

  it("reads agent route data from the query result", async () => {
    const route: AgentRouteData = {
      preview: {
        agent_id: "agent-1",
        agent_name: "Builder",
        tier_source: "override",
        effective_tier: "frontier",
        fallback_chain: [],
        missing: [],
        degraded: false,
      },
      overrides: { tier_source: "override", tier: "frontier" },
    };
    routingMocks.getAgentRoute.mockResolvedValue(route);

    const { result } = renderHook(() => useAgentRoute("agent-1"), {
      wrapper: createQueryWrapper(),
    });

    await waitFor(() => expect(result.current.data).toEqual(route));
  });
});

function createQueryWrapper() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

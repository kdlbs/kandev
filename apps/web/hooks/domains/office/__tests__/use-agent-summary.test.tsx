import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import type { AgentSummaryResponse } from "@/lib/api/domains/office-runs-api";
import { useAgentSummary } from "../use-agent-summary";

const getAgentSummaryMock = vi.hoisted(() => vi.fn());
const wsHandlers = vi.hoisted(
  () => new Map<string, (message: { payload: Record<string, unknown> }) => void>(),
);
const wsOnMock = vi.hoisted(() =>
  vi.fn((event: string, handler: (message: { payload: Record<string, unknown> }) => void) => {
    wsHandlers.set(event, handler);
    return () => wsHandlers.delete(event);
  }),
);

vi.mock("@/lib/api/domains/office-runs-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-runs-api")>(
    "@/lib/api/domains/office-runs-api",
  );
  return { ...actual, getAgentSummary: getAgentSummaryMock };
});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ on: wsOnMock }),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  wsHandlers.clear();
});

const AGENT_ID = "agent-1";

function makeSummary(successRateLen = 0): AgentSummaryResponse {
  return {
    latest_run: null,
    run_activity: [],
    tasks_by_priority: [],
    tasks_by_status: [],
    // success_rate length is a cheap structural marker so TQ's structural
    // sharing returns a NEW object reference after a refetch (it preserves
    // the old reference when the payload is deeply equal).
    success_rate: Array.from({ length: successRateLen }, (_, i) => ({ day: String(i) })),
    recent_tasks: [],
    cost_aggregate: null,
    recent_run_costs: [],
  } as unknown as AgentSummaryResponse;
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={createTestQueryClient()}>{children}</QueryClientProvider>;
}

describe("useAgentSummary", () => {
  it("returns the SSR snapshot immediately via initialData", () => {
    const initial = makeSummary();
    const { result } = renderHook(() => useAgentSummary(AGENT_ID, initial), { wrapper });
    expect(result.current.summary).toBe(initial);
  });

  it("subscribes to office WS events and refetches when one fires", async () => {
    const initial = makeSummary(0);
    getAgentSummaryMock.mockResolvedValue(makeSummary(2));
    const { result } = renderHook(() => useAgentSummary(AGENT_ID, initial), { wrapper });
    expect(result.current.summary.success_rate).toHaveLength(0);

    // It registered handlers for the summary-affecting office events.
    expect(wsOnMock).toHaveBeenCalled();
    expect(wsHandlers.has("office.run.processed")).toBe(true);
    expect(wsHandlers.has("session.state_changed")).toBe(true);

    // Firing one invalidates the query, triggering a refetch that swaps in
    // the freshly-fetched summary.
    wsHandlers.get("office.run.processed")?.({ payload: {} });
    await waitFor(() => expect(getAgentSummaryMock).toHaveBeenCalledWith(AGENT_ID, undefined));
    await waitFor(() => expect(result.current.summary.success_rate).toHaveLength(2));
  });
});

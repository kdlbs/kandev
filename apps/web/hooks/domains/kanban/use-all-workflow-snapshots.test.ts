/**
 * Tests for the TQ-based useAllWorkflowSnapshots hook.
 *
 * The hook now:
 * 1. Calls `useQuery(multiKanbanQueryOptions(workspaceId))` to populate the TQ cache.
 * 2. Calls `queryClient.invalidateQueries(qk.kanban.multi())` when workspaceId changes.
 *
 * We test the invalidation behavior; the fetch itself is handled by TQ and
 * tested in bridge tests.
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { qk } from "@/lib/query/keys";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock("@/lib/query/query-options/kanban", () => ({
  multiKanbanQueryOptions: (workspaceId: string) => ({
    queryKey: qk.kanban.multi(),
    queryFn: vi.fn().mockResolvedValue({ snapshots: {} }),
    enabled: !!workspaceId,
    staleTime: 30_000,
  }),
}));

import { useAllWorkflowSnapshots } from "./use-all-workflow-snapshots";

function createWrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

describe("useAllWorkflowSnapshots — TQ-based", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders without error when workspaceId is null", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    expect(() => {
      renderHook(() => useAllWorkflowSnapshots(null), { wrapper: createWrapper(qc) });
    }).not.toThrow();
  });

  it("renders without error when workspaceId is provided", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    expect(() => {
      renderHook(() => useAllWorkflowSnapshots("ws-1"), { wrapper: createWrapper(qc) });
    }).not.toThrow();
  });

  it("returns void (no return value)", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useAllWorkflowSnapshots("ws-1"), {
      wrapper: createWrapper(qc),
    });
    expect(result.current).toBeUndefined();
  });

  it("invalidates the kanban multi cache when workspaceId changes", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    const { rerender } = renderHook(
      ({ wsId }: { wsId: string | null }) => useAllWorkflowSnapshots(wsId),
      { wrapper: createWrapper(qc), initialProps: { wsId: "ws-1" } },
    );

    // Wait for the effect from initial mount
    await act(async () => {
      await Promise.resolve();
    });

    const callsBefore = invalidateSpy.mock.calls.length;

    // Change workspace
    rerender({ wsId: "ws-2" });

    await act(async () => {
      await Promise.resolve();
    });

    // Should have called invalidateQueries again after the workspace change
    expect(invalidateSpy.mock.calls.length).toBeGreaterThan(callsBefore);

    // Each call should target the kanban multi key
    const kanbanMultiKey = qk.kanban.multi();
    const hasKanbanCall = invalidateSpy.mock.calls.some(
      (call) =>
        JSON.stringify((call[0] as { queryKey?: unknown })?.queryKey) ===
        JSON.stringify(kanbanMultiKey),
    );
    expect(hasKanbanCall).toBe(true);
  });
});

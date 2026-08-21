import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, waitFor } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { CostOverview } from "./cost-overview";

// Hoisted mock so the getCostsBreakdown import is replaced before the
// component imports it. Tests configure the mock per-case.
const getCostsBreakdownMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return {
    ...actual,
    getCostsBreakdown: getCostsBreakdownMock,
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const EMPTY_BREAKDOWN = {
  total_subcents: 0,
  by_agent: [],
  by_project: [],
  by_model: [],
  by_provider: [],
};

// Renders as a StateProvider child purely to hand the real store instance
// back to the test, so it can bump office.refetchTriggers the same way the
// WS handler does and let real zustand reactivity re-render CostOverview.
function StoreCapture({ storeRef }: { storeRef: { current: StoreApi<AppState> | null } }) {
  storeRef.current = useAppStoreApi();
  return null;
}

describe("CostOverview", () => {
  it("refetches the cost breakdown when the costs refetch trigger fires", async () => {
    getCostsBreakdownMock.mockResolvedValue(EMPTY_BREAKDOWN);
    const storeRef: { current: StoreApi<AppState> | null } = { current: null };

    render(
      <StateProvider initialState={{ workspaces: { activeId: "ws-1", items: [] } }}>
        <StoreCapture storeRef={storeRef} />
        <CostOverview workspaceId="ws-1" />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(getCostsBreakdownMock).toHaveBeenCalledTimes(1);
    });

    // Simulates the WS handler bumping "costs" (office.cost.recorded, or
    // office.task.updated with fields: ["project_id"]).
    act(() => {
      storeRef.current!.getState().setOfficeRefetchTrigger("costs");
    });

    await waitFor(() => {
      expect(getCostsBreakdownMock).toHaveBeenCalledTimes(2);
    });
  });

  it("does not refetch on an unrelated refetch trigger", async () => {
    getCostsBreakdownMock.mockResolvedValue(EMPTY_BREAKDOWN);
    const storeRef: { current: StoreApi<AppState> | null } = { current: null };

    render(
      <StateProvider initialState={{ workspaces: { activeId: "ws-1", items: [] } }}>
        <StoreCapture storeRef={storeRef} />
        <CostOverview workspaceId="ws-1" />
      </StateProvider>,
    );

    await waitFor(() => {
      expect(getCostsBreakdownMock).toHaveBeenCalledTimes(1);
    });

    act(() => {
      storeRef.current!.getState().setOfficeRefetchTrigger("dashboard");
    });

    // Give any (incorrect) refetch a chance to fire before asserting it didn't.
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(getCostsBreakdownMock).toHaveBeenCalledTimes(1);
  });
});

import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const usageApi = vi.hoisted(() => ({
  getSessionUsageTotals: vi.fn(),
  getSessionUsageTurn: vi.fn(),
  listSessionUsageTurns: vi.fn(),
}));

const state = vi.hoisted(() => ({
  connection: { status: "connected" },
  usageInvalidation: { bySessionId: { "session-1": 0 } },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (current: typeof state) => unknown) => selector(state),
}));

vi.mock("@/lib/api/domains/conversation-usage-api", () => usageApi);

import { useConversationUsage } from "./use-conversation-usage";

beforeEach(() => {
  vi.clearAllMocks();
  state.connection.status = "connected";
  state.usageInvalidation.bySessionId["session-1"] = 0;
  usageApi.getSessionUsageTotals.mockResolvedValue({ event_count: 3, total_tokens: 9 });
  usageApi.listSessionUsageTurns.mockResolvedValue({ turns: [], total: 0 });
});

describe("panel-scoped conversation usage", () => {
  it("loads usage only while detail demand is active and retains it when hidden", async () => {
    const { result, rerender } = renderHook(
      ({ detailActive }: { detailActive: boolean }) =>
        useConversationUsage("task-1", "session-1", detailActive),
      { initialProps: { detailActive: false } },
    );

    expect(usageApi.getSessionUsageTotals).not.toHaveBeenCalled();
    expect(usageApi.listSessionUsageTurns).not.toHaveBeenCalled();

    rerender({ detailActive: true });
    await waitFor(() => expect(result.current.totals).toEqual({ event_count: 3, total_tokens: 9 }));
    expect(usageApi.getSessionUsageTotals).toHaveBeenCalledTimes(1);
    expect(usageApi.listSessionUsageTurns).toHaveBeenCalledTimes(1);

    rerender({ detailActive: false });
    state.usageInvalidation.bySessionId["session-1"] += 1;
    rerender({ detailActive: false });
    await act(async () => result.current.refresh());

    expect(result.current.totals).toEqual({ event_count: 3, total_tokens: 9 });
    expect(usageApi.getSessionUsageTotals).toHaveBeenCalledTimes(1);
    expect(usageApi.listSessionUsageTurns).toHaveBeenCalledTimes(1);
  });
});

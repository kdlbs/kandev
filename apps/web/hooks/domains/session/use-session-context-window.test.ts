import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const subscribeSession = vi.fn(() => vi.fn());
const setContextWindow = vi.fn();
let storeState: Record<string, unknown>;

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ subscribeSession }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(storeState),
}));

import { useSessionContextWindow } from "./use-session-context-window";

describe("useSessionContextWindow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    storeState = {
      contextWindow: { bySessionId: {} },
      taskSessions: {
        items: {
          "session-1": {
            metadata: {
              context_window: {
                size: 100_000,
                used: 42_000,
                remaining: 58_000,
                efficiency: 42,
                source: "acp",
              },
              context_compaction_count: 4,
            },
          },
        },
      },
      setContextWindow,
      connection: { status: "disconnected" },
    };
  });

  it("hydrates the persisted compaction count with session context metadata", async () => {
    renderHook(() => useSessionContextWindow("session-1"));

    await waitFor(() =>
      expect(setContextWindow).toHaveBeenCalledWith("session-1", {
        size: 100_000,
        used: 42_000,
        remaining: 58_000,
        efficiency: 42,
        compactionCount: 4,
        source: "acp",
      }),
    );
  });
});

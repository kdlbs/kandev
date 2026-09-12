import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import type { HydrationState } from "@/lib/state/store";

const listClarificationInboxMock = vi.fn();
vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  listClarificationInbox: (...args: unknown[]) => listClarificationInboxMock(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

import { useNeedsYouInboxController } from "./use-needs-you-inbox-controller";

const WORKSPACE_ID = "w1";

function page(overrides: Partial<Awaited<ReturnType<typeof listClarificationInboxMock>>> = {}) {
  return {
    bundles: [],
    count: 0,
    hidden_count: 0,
    next_snooze_expiry: null,
    ...overrides,
  };
}

function initialState(enabled: boolean): HydrationState {
  return {
    features: { ...defaultFeatureFlags, needsYouInbox: enabled },
    workspaces: { activeId: WORKSPACE_ID, byId: {}, allIds: [] },
  } as unknown as HydrationState;
}

function renderController(enabled = true) {
  return renderHook(
    () => {
      useNeedsYouInboxController();
      return useAppStoreApi();
    },
    {
      wrapper: ({ children }) => (
        <StateProvider initialState={initialState(enabled)}>{children}</StateProvider>
      ),
    },
  );
}

beforeEach(() => {
  listClarificationInboxMock.mockReset();
  listClarificationInboxMock.mockResolvedValue(page());
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useNeedsYouInboxController", () => {
  it("reads on mount for the active workspace when the flag is enabled", async () => {
    renderController(true);

    await waitFor(() => expect(listClarificationInboxMock).toHaveBeenCalledWith(WORKSPACE_ID));
  });

  it("never reads when the flag is disabled", async () => {
    renderController(false);

    await act(async () => {
      await Promise.resolve();
    });
    expect(listClarificationInboxMock).not.toHaveBeenCalled();
  });

  it("applies a successful read into the count slice", async () => {
    listClarificationInboxMock.mockResolvedValue(
      page({ count: 2, bundles: [], hidden_count: 1, next_snooze_expiry: null }),
    );
    const { result } = renderController(true);

    await waitFor(() => {
      const state = result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID];
      expect(state?.status).toBe("ready");
      expect(state?.count).toBe(2);
      expect(state?.hiddenCount).toBe(1);
    });
  });

  it("re-reads when the WS refresh trigger tick is bumped", async () => {
    const { result } = renderController(true);
    await waitFor(() => expect(listClarificationInboxMock).toHaveBeenCalledTimes(1));

    act(() => {
      result.current.getState().bumpNeedsYouInboxRefreshTick();
    });

    await waitFor(() => expect(listClarificationInboxMock).toHaveBeenCalledTimes(2));
  });

  it("re-reads once at the periodic interval while the tab stays visible", async () => {
    vi.useFakeTimers();
    renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });

    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
  });

  it("arms a snooze-expiry timer from the read response and re-reads once it fires", async () => {
    vi.useFakeTimers();
    const soon = new Date(Date.now() + 10_000).toISOString();
    listClarificationInboxMock.mockResolvedValueOnce(page({ next_snooze_expiry: soon }));
    listClarificationInboxMock.mockResolvedValue(page());

    renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });

    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
  });
});

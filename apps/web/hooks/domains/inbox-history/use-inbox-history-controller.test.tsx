import { act, cleanup, fireEvent, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import type { HydrationState } from "@/lib/state/store";

const listInboxHistoryMock = vi.fn();
vi.mock("@/lib/api/domains/inbox-history-api", () => ({
  listInboxHistory: (...args: unknown[]) => listInboxHistoryMock(...args),
}));

import { useInboxHistoryController } from "./use-inbox-history-controller";

const WORKSPACE_ID = "w1";

function page(overrides: Partial<Awaited<ReturnType<typeof listInboxHistoryMock>>> = {}) {
  return { bundles: [], count: 0, total: 0, ...overrides };
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
      useInboxHistoryController();
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
  listInboxHistoryMock.mockReset();
  listInboxHistoryMock.mockResolvedValue(page());
});

afterEach(() => {
  cleanup();
});

describe("useInboxHistoryController", () => {
  it("reads on mount for the active workspace when the flag is enabled", async () => {
    renderController(true);

    await waitFor(() => expect(listInboxHistoryMock).toHaveBeenCalledWith(WORKSPACE_ID));
  });

  it("never reads when the flag is disabled", async () => {
    renderController(false);

    await act(async () => {
      await Promise.resolve();
    });
    expect(listInboxHistoryMock).not.toHaveBeenCalled();
  });

  it("applies a successful page to the workspace's slice state", async () => {
    listInboxHistoryMock.mockResolvedValue(page({ bundles: [], total: 3 }));
    const { result } = renderController(true);

    await waitFor(() => {
      const state = result.current.getState().inboxHistory.byWorkspaceId[WORKSPACE_ID];
      expect(state?.status).toBe("ready");
      expect(state?.total).toBe(3);
    });
  });

  it("applies an error state when the read rejects", async () => {
    listInboxHistoryMock.mockRejectedValue(new Error("boom"));
    const { result } = renderController(true);

    await waitFor(() => {
      expect(result.current.getState().inboxHistory.byWorkspaceId[WORKSPACE_ID]?.status).toBe(
        "error",
      );
    });
  });

  it("re-reads when the browser regains foreground", async () => {
    renderController(true);
    await waitFor(() => expect(listInboxHistoryMock).toHaveBeenCalledTimes(1));

    listInboxHistoryMock.mockClear();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => "visible",
    });
    fireEvent(document, new Event("visibilitychange"));

    await waitFor(() => expect(listInboxHistoryMock).toHaveBeenCalledTimes(1));
  });

  it("subscribes to no WebSocket client (AC .22: point-in-time projection only)", async () => {
    const wsModule = await import("@/lib/ws/connection");
    const spy = vi.spyOn(wsModule, "getWebSocketClient");
    renderController(true);

    await waitFor(() => expect(listInboxHistoryMock).toHaveBeenCalled());
    expect(spy).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});

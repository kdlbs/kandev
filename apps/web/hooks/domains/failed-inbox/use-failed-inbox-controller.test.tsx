import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import type { HydrationState } from "@/lib/state/store";

const listFailedInboxMock = vi.fn();
vi.mock("@/lib/api/domains/failed-inbox-api", () => ({
  listFailedInbox: (...args: unknown[]) => listFailedInboxMock(...args),
}));

import { useFailedInboxController } from "./use-failed-inbox-controller";

const WORKSPACE_ID = "w1";

function page(overrides: Partial<Awaited<ReturnType<typeof listFailedInboxMock>>> = {}) {
  return { rows: [], count: 0, truncated: false, ...overrides };
}

function initialState(enabled: boolean, workspaceId = WORKSPACE_ID): HydrationState {
  return {
    features: { ...defaultFeatureFlags, needsYouInbox: enabled },
    workspaces: { activeId: workspaceId, byId: {}, allIds: [] },
  } as unknown as HydrationState;
}

function renderController(selectedTab: "needs-you" | "failed" = "needs-you", enabled = true) {
  let rerenderTab: (tab: "needs-you" | "failed") => void = () => {};
  const view = renderHook(
    ({ tab }: { tab: "needs-you" | "failed" }) => {
      useFailedInboxController(tab);
      return useAppStoreApi();
    },
    {
      initialProps: { tab: selectedTab },
      wrapper: ({ children }) => (
        <StateProvider initialState={initialState(enabled)}>{children}</StateProvider>
      ),
    },
  );
  rerenderTab = (tab) => view.rerender({ tab });
  return { ...view, rerenderTab };
}

beforeEach(() => {
  listFailedInboxMock.mockReset();
  listFailedInboxMock.mockResolvedValue(page());
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useFailedInboxController", () => {
  it("reads on mount for the active workspace regardless of selected tab", async () => {
    renderController("needs-you");
    await waitFor(() => expect(listFailedInboxMock).toHaveBeenCalledWith(WORKSPACE_ID));
  });

  it("never reads when the Inbox feature flag is disabled", async () => {
    renderController("needs-you", false);
    await act(async () => {
      await Promise.resolve();
    });
    expect(listFailedInboxMock).not.toHaveBeenCalled();
  });

  it("applies a successful read into the failed-inbox slice", async () => {
    listFailedInboxMock.mockResolvedValue(
      page({
        count: 1,
        truncated: false,
        rows: [{ task_id: "t1", title: "T", workspace_id: WORKSPACE_ID, origin: "", reason: "" }],
      }),
    );
    const { result } = renderController("failed");

    await waitFor(() => {
      const state = result.current.getState().failedInbox.byWorkspaceId[WORKSPACE_ID];
      expect(state?.status).toBe("ready");
      expect(state?.count).toBe(1);
      expect(state?.rows).toHaveLength(1);
    });
  });

  it("applies an errored read into the failed-inbox slice", async () => {
    listFailedInboxMock.mockRejectedValue(new Error("boom"));
    const { result } = renderController("needs-you");

    await waitFor(() => {
      const state = result.current.getState().failedInbox.byWorkspaceId[WORKSPACE_ID];
      expect(state?.status).toBe("error");
    });
  });

  it("re-reads when the selected tab changes, even though it already read at mount", async () => {
    const { rerenderTab } = renderController("needs-you");
    await waitFor(() => expect(listFailedInboxMock).toHaveBeenCalledTimes(1));

    rerenderTab("failed");

    await waitFor(() => expect(listFailedInboxMock).toHaveBeenCalledTimes(2));
  });

  it("re-reads once at the periodic 60s interval while the tab stays visible, regardless of selected tab", async () => {
    vi.useFakeTimers();
    renderController("needs-you");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listFailedInboxMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });

    expect(listFailedInboxMock).toHaveBeenCalledTimes(2);
  });

  it("does not re-read at the periodic interval while the tab is hidden", async () => {
    vi.useFakeTimers();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => "hidden",
    });
    renderController("needs-you");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    listFailedInboxMock.mockClear();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });

    expect(listFailedInboxMock).not.toHaveBeenCalled();
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => "visible",
    });
  });
});

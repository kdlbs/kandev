import { act, cleanup, renderHook } from "@testing-library/react";
import type { DockviewPanelApi } from "dockview-react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  client: { refreshSessionData: vi.fn() },
  getWebSocketClient: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: mocks.getWebSocketClient,
}));

import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { useResyncGitStatusOnTabActivate } from "./use-resync-git-status-on-tab-activate";

type FakeApiHandle = {
  api: { isActive: boolean; onDidActiveChange: FakePanelApi["onDidActiveChange"] };
  fireActiveChange: (isActive: boolean) => void;
};

type FakePanelApi = {
  isActive: boolean;
  onDidActiveChange: (listener: (event: { isActive: boolean }) => void) => {
    dispose: () => void;
  };
};

function acquirePanel(panelId: string, initialIsActive: boolean): FakeApiHandle {
  let listener: ((event: { isActive: boolean }) => void) | null = null;
  const api: FakePanelApi = {
    isActive: initialIsActive,
    onDidActiveChange: (nextListener) => {
      listener = nextListener;
      return { dispose: () => (listener = null) };
    },
  };
  panelPortalManager.acquire(panelId, "changes", {}, api as unknown as DockviewPanelApi);
  return {
    api,
    fireActiveChange: (isActive) => {
      api.isActive = isActive;
      listener?.({ isActive });
    },
  };
}

describe("useResyncGitStatusOnTabActivate", () => {
  beforeEach(() => {
    mocks.getWebSocketClient.mockReturnValue(mocks.client);
  });

  afterEach(() => {
    cleanup();
    for (const panelId of panelPortalManager.ids()) panelPortalManager.release(panelId);
    vi.clearAllMocks();
  });

  it("refreshes an already-active panel when it mounts", () => {
    acquirePanel("changes", true);

    renderHook(() => useResyncGitStatusOnTabActivate("changes", "session-1"));

    expect(mocks.client.refreshSessionData).toHaveBeenCalledWith("session-1");
  });

  it("refreshes when an inactive panel becomes active", () => {
    const panel = acquirePanel("changes", false);
    renderHook(() => useResyncGitStatusOnTabActivate("changes", "session-1"));

    act(() => panel.fireActiveChange(true));

    expect(mocks.client.refreshSessionData).toHaveBeenCalledOnce();
    expect(mocks.client.refreshSessionData).toHaveBeenCalledWith("session-1");
  });

  it("stops listening after unmount", () => {
    const panel = acquirePanel("changes", false);
    const { unmount } = renderHook(() => useResyncGitStatusOnTabActivate("changes", "session-1"));
    unmount();

    act(() => panel.fireActiveChange(true));

    expect(mocks.client.refreshSessionData).not.toHaveBeenCalled();
  });
});

import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { DockviewApi, DockviewPanelApi } from "dockview-react";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { usePanelActive } from "./use-panel-active";

const livePanels = new Map<string, DockviewPanelApi>();
let previousApi: DockviewApi | null;
beforeEach(() => {
  previousApi = useDockviewStore.getState().api;
  useDockviewStore.setState({
    api: {
      getPanel: (id: string) => {
        const api = livePanels.get(id);
        return api ? { api } : undefined;
      },
    } as DockviewApi,
  });
});

afterEach(() => {
  cleanup();
  for (const id of panelPortalManager.ids()) panelPortalManager.release(id);
  livePanels.clear();
  useDockviewStore.setState({ api: previousApi });
});

type FakePanelApi = {
  isActive: boolean;
  isVisible: boolean;
  onDidActiveChange: (cb: (event: { isActive: boolean }) => void) => { dispose: () => void };
  onDidVisibilityChange: (cb: (event: { isVisible: boolean }) => void) => {
    dispose: () => void;
  };
};

type FakeApiHandle = {
  fakeApi: FakePanelApi;
  fireActiveChange: (isActive: boolean) => void;
  fireVisibilityChange: (isVisible: boolean) => void;
};

function makeFakeApi(initialIsActive: boolean, initialIsVisible = initialIsActive): FakeApiHandle {
  let activeListener: ((event: { isActive: boolean }) => void) | null = null;
  let visibilityListener: ((event: { isVisible: boolean }) => void) | null = null;
  const fakeApi: FakePanelApi = {
    isActive: initialIsActive,
    isVisible: initialIsVisible,
    onDidActiveChange: (cb) => {
      activeListener = cb;
      return { dispose: () => (activeListener = null) };
    },
    onDidVisibilityChange: (cb) => {
      visibilityListener = cb;
      return { dispose: () => (visibilityListener = null) };
    },
  };
  return {
    fakeApi,
    fireActiveChange: (isActive: boolean) => {
      fakeApi.isActive = isActive;
      activeListener?.({ isActive });
    },
    fireVisibilityChange: (isVisible: boolean) => {
      fakeApi.isVisible = isVisible;
      visibilityListener?.({ isVisible });
    },
  };
}

function acquirePanel(
  panelId: string,
  initialIsActive: boolean,
  initialIsVisible = initialIsActive,
): FakeApiHandle {
  const handle = makeFakeApi(initialIsActive, initialIsVisible);
  // Test double: only the subset of DockviewPanelApi this hook actually
  // reads/calls is implemented. Unchecked cast is intentional here — the
  // full interface has 30+ unrelated members no test in this file exercises.
  const dockviewApi = handle.fakeApi as unknown as DockviewPanelApi;
  livePanels.set(panelId, dockviewApi);
  panelPortalManager.acquire(panelId, "chat", {}, dockviewApi);
  return handle;
}

describe("usePanelActive", () => {
  it("returns false before a portal entry/api has been registered for the panel", () => {
    const { result } = renderHook(() => usePanelActive("panel-unregistered"));
    expect(result.current).toBe(false);
  });

  it("reflects the panel's initial isVisible state once registered", () => {
    acquirePanel("panel-active", true);
    const { result } = renderHook(() => usePanelActive("panel-active"));
    expect(result.current).toBe(true);

    acquirePanel("panel-inactive", false);
    const { result: result2 } = renderHook(() => usePanelActive("panel-inactive"));
    expect(result2.current).toBe(false);
  });

  it("treats a selected panel as visible when another Dockview group owns focus", () => {
    const handle = acquirePanel("panel-visible-unfocused", false, true);
    const { result } = renderHook(() => usePanelActive("panel-visible-unfocused"));

    expect(result.current).toBe(true);

    // The hook subscribes to onDidVisibilityChange, not onDidActiveChange.
    // Firing active-change events is a no-op; the result must not move.
    act(() => handle.fireActiveChange(true));
    act(() => handle.fireActiveChange(false));
    expect(result.current).toBe(true);

    act(() => handle.fireVisibilityChange(false));
    expect(result.current).toBe(false);
  });

  it("updates when the panel's visible tab changes via onDidVisibilityChange", () => {
    const handle = acquirePanel("panel-toggle", false);
    const { result } = renderHook(() => usePanelActive("panel-toggle"));
    expect(result.current).toBe(false);

    act(() => handle.fireVisibilityChange(true));
    expect(result.current).toBe(true);

    act(() => handle.fireVisibilityChange(false));
    expect(result.current).toBe(false);
  });

  it("falls back to false again if the panel is released", () => {
    const PANEL_ID = "panel-released";
    acquirePanel(PANEL_ID, true);
    const { result, rerender } = renderHook(({ id }: { id: string }) => usePanelActive(id), {
      initialProps: { id: PANEL_ID },
    });
    expect(result.current).toBe(true);

    panelPortalManager.release(PANEL_ID);
    rerender({ id: PANEL_ID });
    expect(result.current).toBe(false);
  });
});

describe("layout-owned panel visibility", () => {
  it("does not expose a transiently selected tab while layout restoration rebuilds panels", () => {
    const restoring = useDockviewStore.getState().isRestoringLayout;
    const panel = acquirePanel("restoring-neighbor", false);
    const { result } = renderHook(() => usePanelActive("restoring-neighbor"));
    try {
      act(() => {
        useDockviewStore.setState({ isRestoringLayout: true });
        panel.fireVisibilityChange(true);
      });
      expect(result.current).toBe(false);
      act(() => {
        panel.fireVisibilityChange(false);
        useDockviewStore.setState({ isRestoringLayout: false });
      });
      expect(result.current).toBe(false);
      act(() => panel.fireVisibilityChange(true));
      expect(result.current).toBe(true);
    } finally {
      act(() => useDockviewStore.setState({ isRestoringLayout: restoring }));
    }
  });

  it("keeps a parked portal unreadable when its disposed API still reports visibility", () => {
    const restoring = useDockviewStore.getState().isRestoringLayout;
    const panelId = "parked-neighbor";
    const panel = acquirePanel(panelId, true);
    const { result } = renderHook(() => usePanelActive(panelId));
    expect(result.current).toBe(true);
    try {
      act(() => {
        useDockviewStore.setState({ isRestoringLayout: true });
        livePanels.delete(panelId);
        panel.fireVisibilityChange(true);
      });
      act(() => useDockviewStore.setState({ isRestoringLayout: false }));
      expect(result.current).toBe(false);
      act(() => {
        acquirePanel(panelId, true);
      });
      expect(result.current).toBe(true);
    } finally {
      act(() => useDockviewStore.setState({ isRestoringLayout: restoring }));
    }
  });

  it("rebinds to the new panel api when Dockview remounts (replaces) an existing portal entry", () => {
    const PANEL_ID = "panel-remount";
    const stale = acquirePanel(PANEL_ID, false);
    const { result } = renderHook(() => usePanelActive(PANEL_ID));
    expect(result.current).toBe(false);

    // Simulate Dockview calling api.fromJSON() during a layout restore: the
    // same panelId is re-acquired with a brand-new DockviewPanelApi,
    // without the old one ever being released.
    const fresh = acquirePanel(PANEL_ID, false);

    act(() => fresh.fireVisibilityChange(true));
    expect(result.current).toBe(true);

    // The stale api's listener must have been torn down — firing it no
    // longer moves the hook (it would incorrectly flip back to false
    // without the resubscribe fix).
    act(() => stale.fireVisibilityChange(false));
    expect(result.current).toBe(true);
  });
});

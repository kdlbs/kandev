import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { DockviewPanelApi } from "dockview-react";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { usePanelActive } from "./use-panel-active";

afterEach(() => {
  cleanup();
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
  panelPortalManager.acquire(panelId, "chat", {}, dockviewApi);
  return handle;
}

describe("usePanelActive", () => {
  afterEach(() => {
    // Release any panels acquired by a test so state doesn't leak across cases.
    for (const id of panelPortalManager.ids()) panelPortalManager.release(id);
  });

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

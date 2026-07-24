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
  onDidActiveChange: (cb: (event: { isActive: boolean }) => void) => { dispose: () => void };
};

type FakeApiHandle = {
  fakeApi: FakePanelApi;
  fireActiveChange: (isActive: boolean) => void;
};

function makeFakeApi(initialIsActive: boolean): FakeApiHandle {
  let listener: ((event: { isActive: boolean }) => void) | null = null;
  const fakeApi: FakePanelApi = {
    isActive: initialIsActive,
    onDidActiveChange: (cb) => {
      listener = cb;
      return { dispose: () => (listener = null) };
    },
  };
  return {
    fakeApi,
    fireActiveChange: (isActive: boolean) => {
      fakeApi.isActive = isActive;
      listener?.({ isActive });
    },
  };
}

function acquirePanel(panelId: string, initialIsActive: boolean): FakeApiHandle {
  const handle = makeFakeApi(initialIsActive);
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

  it("reflects the panel's initial isActive state once registered", () => {
    acquirePanel("panel-active", true);
    const { result } = renderHook(() => usePanelActive("panel-active"));
    expect(result.current).toBe(true);

    acquirePanel("panel-inactive", false);
    const { result: result2 } = renderHook(() => usePanelActive("panel-inactive"));
    expect(result2.current).toBe(false);
  });

  it("updates when the panel's active tab changes via onDidActiveChange", () => {
    const handle = acquirePanel("panel-toggle", false);
    const { result } = renderHook(() => usePanelActive("panel-toggle"));
    expect(result.current).toBe(false);

    act(() => handle.fireActiveChange(true));
    expect(result.current).toBe(true);

    act(() => handle.fireActiveChange(false));
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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import type { DockviewApi } from "dockview-react";
import { useDockviewUnmountCleanup } from "./dockview-desktop-layout-hooks";

const mocks = vi.hoisted(() => ({
  currentApi: null as DockviewApi | null,
  setApi: vi.fn(),
  releaseAll: vi.fn(),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: { releaseAll: mocks.releaseAll },
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(vi.fn(), {
    getState: () => ({ api: mocks.currentApi, setApi: mocks.setApi }),
  }),
}));

function refs(api: DockviewApi | null, dispose = vi.fn()) {
  return {
    timer: { current: null as ReturnType<typeof setTimeout> | null },
    disposers: { current: [dispose] },
    api: { current: api },
    dispose,
  };
}

describe("Dockview unmount ownership", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.currentApi = null;
    mocks.setApi.mockImplementation((api: DockviewApi | null) => {
      mocks.currentApi = api;
    });
  });

  it("releases the departed API and portals with its layout listeners", () => {
    const owned = {} as DockviewApi;
    mocks.currentApi = owned;
    const state = refs(owned);
    const hook = renderHook(() =>
      useDockviewUnmountCleanup(state.timer, state.disposers, state.api),
    );
    hook.unmount();
    expect(state.dispose).toHaveBeenCalledOnce();
    expect(mocks.releaseAll).toHaveBeenCalledOnce();
    expect(mocks.setApi).toHaveBeenCalledWith(null);
    expect(mocks.currentApi).toBeNull();
  });

  it("cancels pending saves when the layout unmounts", () => {
    vi.useFakeTimers();
    try {
      const save = vi.fn();
      const state = refs(null);
      // Browser tests use numeric timer handles; the shared ref also supports Node timers.
      state.timer.current = setTimeout(save, 100) as unknown as ReturnType<typeof setTimeout>;
      const hook = renderHook(() =>
        useDockviewUnmountCleanup(state.timer, state.disposers, state.api),
      );
      hook.unmount();
      vi.runAllTimers();
      expect(save).not.toHaveBeenCalled();
      expect(state.disposers.current).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not clear the API or portals acquired by a replacement layout", () => {
    const owned = {} as DockviewApi;
    const replacement = {} as DockviewApi;
    mocks.currentApi = owned;
    const state = refs(owned);
    const hook = renderHook(() =>
      useDockviewUnmountCleanup(state.timer, state.disposers, state.api),
    );
    mocks.currentApi = replacement;
    hook.unmount();
    expect(state.dispose).toHaveBeenCalledOnce();
    expect(mocks.releaseAll).not.toHaveBeenCalled();
    expect(mocks.setApi).not.toHaveBeenCalled();
    expect(mocks.currentApi).toBe(replacement);
  });

  it("does not release another layout when this layout never became ready", () => {
    const replacement = {} as DockviewApi;
    mocks.currentApi = replacement;
    const state = refs(null);
    const hook = renderHook(() =>
      useDockviewUnmountCleanup(state.timer, state.disposers, state.api),
    );
    hook.unmount();
    expect(mocks.releaseAll).not.toHaveBeenCalled();
    expect(mocks.setApi).not.toHaveBeenCalled();
    expect(mocks.currentApi).toBe(replacement);
  });
});

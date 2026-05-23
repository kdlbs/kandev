import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useMinimizedGroupSync } from "./dockview-minimize-sync";

const MINIMIZED_ATTR = "data-kandev-minimized";

type FakeListener<T> = (event: T) => void;

type FakeGroup = {
  id: string;
  element: HTMLElement;
  api: {
    width: number;
    height: number;
    setSize: ReturnType<typeof vi.fn>;
  };
};

type FakePanel = { group: FakeGroup };

type FakeApi = {
  groups: FakeGroup[];
  emit: {
    addGroup: (g: FakeGroup) => void;
    removeGroup: (g: FakeGroup) => void;
    addPanel: (panel: FakePanel) => void;
  };
};

function makeFakeApi(initialGroups: FakeGroup[] = []): { api: DockviewApi; fake: FakeApi } {
  const addGroupListeners: Array<FakeListener<FakeGroup>> = [];
  const removeGroupListeners: Array<FakeListener<FakeGroup>> = [];
  const addPanelListeners: Array<FakeListener<FakePanel>> = [];

  const api = {
    groups: [...initialGroups],
    onDidAddGroup: (cb: FakeListener<FakeGroup>) => {
      addGroupListeners.push(cb);
      return { dispose: () => addGroupListeners.splice(addGroupListeners.indexOf(cb), 1) };
    },
    onDidRemoveGroup: (cb: FakeListener<FakeGroup>) => {
      removeGroupListeners.push(cb);
      return { dispose: () => removeGroupListeners.splice(removeGroupListeners.indexOf(cb), 1) };
    },
    onDidAddPanel: (cb: FakeListener<FakePanel>) => {
      addPanelListeners.push(cb);
      return { dispose: () => addPanelListeners.splice(addPanelListeners.indexOf(cb), 1) };
    },
  } as unknown as DockviewApi & { groups: FakeGroup[] };

  const fake: FakeApi = {
    groups: (api as unknown as { groups: FakeGroup[] }).groups,
    emit: {
      addGroup: (g) => {
        fake.groups.push(g);
        addGroupListeners.slice().forEach((cb) => cb(g));
      },
      removeGroup: (g) => {
        const idx = fake.groups.indexOf(g);
        if (idx >= 0) fake.groups.splice(idx, 1);
        removeGroupListeners.slice().forEach((cb) => cb(g));
      },
      addPanel: (p) => addPanelListeners.slice().forEach((cb) => cb(p)),
    },
  };

  return { api: api as DockviewApi, fake };
}

function makeGroup(id: string, opts: { width?: number; height?: number } = {}): FakeGroup {
  return {
    id,
    element: document.createElement("div"),
    api: {
      width: opts.width ?? 800,
      height: opts.height ?? 600,
      setSize: vi.fn(),
    },
  };
}

describe("useMinimizedGroupSync", () => {
  beforeEach(() => {
    useDockviewStore.setState({ minimizedGroupIds: new Set<string>() });
  });
  afterEach(() => {
    useDockviewStore.setState({ minimizedGroupIds: new Set<string>() });
  });

  it("stamps data-kandev-minimized=false on every group at mount", () => {
    const groupA = makeGroup("A");
    const groupB = makeGroup("B");
    const { api } = makeFakeApi([groupA, groupB]);

    renderHook(() => useMinimizedGroupSync(api));

    expect(groupA.element.getAttribute(MINIMIZED_ATTR)).toBe("false");
    expect(groupB.element.getAttribute(MINIMIZED_ATTR)).toBe("false");
  });

  it("does nothing when api is null", () => {
    const { result } = renderHook(() => useMinimizedGroupSync(null));
    expect(result.current).toBeUndefined();
  });

  it("on minimize: sets data-attribute=true, records prior size, calls setSize(0,0)", () => {
    const groupA = makeGroup("A", { width: 800, height: 600 });
    const { api } = makeFakeApi([groupA]);
    renderHook(() => useMinimizedGroupSync(api));

    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });

    expect(groupA.element.getAttribute(MINIMIZED_ATTR)).toBe("true");
    expect(groupA.api.setSize).toHaveBeenCalledWith({ width: 0, height: 0 });
  });

  it("on restore: replays the recorded prior size and resets data-attribute", () => {
    const groupA = makeGroup("A", { width: 800, height: 600 });
    const { api } = makeFakeApi([groupA]);
    renderHook(() => useMinimizedGroupSync(api));

    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    expect(groupA.api.setSize).toHaveBeenLastCalledWith({ width: 0, height: 0 });

    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    expect(groupA.element.getAttribute(MINIMIZED_ATTR)).toBe("false");
    expect(groupA.api.setSize).toHaveBeenLastCalledWith({ width: 800, height: 600 });
  });

  it("auto-restores a minimized group when a new panel is added to it", () => {
    const groupA = makeGroup("A");
    const { api, fake } = makeFakeApi([groupA]);
    renderHook(() => useMinimizedGroupSync(api));

    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    expect(useDockviewStore.getState().minimizedGroupIds.has("A")).toBe(true);

    act(() => {
      fake.emit.addPanel({ group: groupA });
    });
    expect(useDockviewStore.getState().minimizedGroupIds.has("A")).toBe(false);
  });

  it("ignores panel-add events for groups that are not minimized", () => {
    const groupA = makeGroup("A");
    const groupB = makeGroup("B");
    const { api, fake } = makeFakeApi([groupA, groupB]);
    renderHook(() => useMinimizedGroupSync(api));

    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    act(() => {
      fake.emit.addPanel({ group: groupB });
    });

    expect(useDockviewStore.getState().minimizedGroupIds.has("A")).toBe(true);
  });
});

describe("useMinimizedGroupSync — group lifecycle", () => {
  beforeEach(() => {
    useDockviewStore.setState({ minimizedGroupIds: new Set<string>() });
  });
  afterEach(() => {
    useDockviewStore.setState({ minimizedGroupIds: new Set<string>() });
  });

  it("evicts a removed group from minimizedGroupIds", () => {
    const groupA = makeGroup("A");
    const { api, fake } = makeFakeApi([groupA]);
    renderHook(() => useMinimizedGroupSync(api));

    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    expect(useDockviewStore.getState().minimizedGroupIds.has("A")).toBe(true);

    act(() => {
      fake.emit.removeGroup(groupA);
    });
    expect(useDockviewStore.getState().minimizedGroupIds.has("A")).toBe(false);
  });

  it("clears recorded prior size when the group is removed (no stale restore on a re-added ID)", () => {
    const groupA = makeGroup("A", { width: 800, height: 600 });
    const { api, fake } = makeFakeApi([groupA]);
    renderHook(() => useMinimizedGroupSync(api));

    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    act(() => {
      fake.emit.removeGroup(groupA);
    });

    // A new group with the same ID later — its size should NOT be reset to the
    // old prior. We re-add and then minimize+restore; restore should use the
    // new group's current width/height, not the deleted one.
    const groupA2 = makeGroup("A", { width: 400, height: 300 });
    act(() => {
      fake.emit.addGroup(groupA2);
    });
    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    expect(groupA2.api.setSize).toHaveBeenLastCalledWith({ width: 400, height: 300 });
  });

  it("syncs newly added groups", () => {
    const groupA = makeGroup("A");
    const { api, fake } = makeFakeApi([groupA]);
    renderHook(() => useMinimizedGroupSync(api));

    const groupB = makeGroup("B");
    act(() => {
      fake.emit.addGroup(groupB);
    });

    expect(groupB.element.getAttribute(MINIMIZED_ATTR)).toBe("false");
  });

  it("does not rewrite data-attribute when unchanged (perf guard)", () => {
    const groupA = makeGroup("A");
    const { api, fake } = makeFakeApi([groupA]);
    renderHook(() => useMinimizedGroupSync(api));

    const setAttrSpy = vi.spyOn(groupA.element, "setAttribute");
    // Trigger a sync that wouldn't change the existing attribute.
    act(() => {
      fake.emit.addGroup(makeGroup("B"));
    });
    expect(setAttrSpy).not.toHaveBeenCalledWith(MINIMIZED_ATTR, "false");
  });

  it("disposes all subscribers on unmount", () => {
    const groupA = makeGroup("A");
    const { api, fake } = makeFakeApi([groupA]);
    const { unmount } = renderHook(() => useMinimizedGroupSync(api));

    unmount();

    // After unmount, store changes must not touch the group element or call setSize.
    const setAttrSpy = vi.spyOn(groupA.element, "setAttribute");
    act(() => {
      useDockviewStore.getState().toggleGroupMinimized("A");
    });
    expect(setAttrSpy).not.toHaveBeenCalled();
    expect(groupA.api.setSize).not.toHaveBeenCalled();

    // And dockview events should no longer mutate the store.
    act(() => {
      fake.emit.removeGroup(groupA);
    });
    // The store still has "A" because the onDidRemoveGroup listener was disposed.
    expect(useDockviewStore.getState().minimizedGroupIds.has("A")).toBe(true);
  });
});

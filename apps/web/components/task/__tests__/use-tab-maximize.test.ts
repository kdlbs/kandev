import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const SIDEBAR_GROUP_ID = "sidebar-group";

const mockMaximizeGroup = vi.fn();
const mockExitMaximizedLayout = vi.fn();

const storeState: {
  preMaximizeLayout: object | null;
  sidebarGroupId: string;
  isRestoringLayout: boolean;
  maximizeGroup: typeof mockMaximizeGroup;
  exitMaximizedLayout: typeof mockExitMaximizedLayout;
} = {
  preMaximizeLayout: null,
  sidebarGroupId: SIDEBAR_GROUP_ID,
  isRestoringLayout: false,
  maximizeGroup: mockMaximizeGroup,
  exitMaximizedLayout: mockExitMaximizedLayout,
};

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: typeof storeState) => unknown) => selector(storeState),
    { getState: () => storeState },
  ),
}));

import { useTabMaximizeOnDoubleClick } from "../use-tab-maximize";

function makeApi(groupId: string | undefined) {
  return { group: groupId === undefined ? undefined : { id: groupId } } as Parameters<
    typeof useTabMaximizeOnDoubleClick
  >[0];
}

function makeEvent() {
  return {
    stopPropagation: vi.fn(),
    preventDefault: vi.fn(),
  } as unknown as React.MouseEvent;
}

describe("useTabMaximizeOnDoubleClick", () => {
  beforeEach(() => {
    mockMaximizeGroup.mockClear();
    mockExitMaximizedLayout.mockClear();
    storeState.preMaximizeLayout = null;
    storeState.sidebarGroupId = SIDEBAR_GROUP_ID;
    storeState.isRestoringLayout = false;
  });

  it("calls maximizeGroup when not currently maximized", () => {
    const { result } = renderHook(() => useTabMaximizeOnDoubleClick(makeApi("group-a")));
    result.current(makeEvent());
    expect(mockMaximizeGroup).toHaveBeenCalledWith("group-a");
    expect(mockExitMaximizedLayout).not.toHaveBeenCalled();
  });

  it("calls exitMaximizedLayout when currently maximized", () => {
    storeState.preMaximizeLayout = { columns: [] };
    const { result } = renderHook(() => useTabMaximizeOnDoubleClick(makeApi("group-a")));
    result.current(makeEvent());
    expect(mockExitMaximizedLayout).toHaveBeenCalledTimes(1);
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
  });

  it("no-ops when groupId is the sidebar group", () => {
    const { result } = renderHook(() => useTabMaximizeOnDoubleClick(makeApi(SIDEBAR_GROUP_ID)));
    result.current(makeEvent());
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
    expect(mockExitMaximizedLayout).not.toHaveBeenCalled();
  });

  it("no-ops while a layout restore is in progress", () => {
    storeState.isRestoringLayout = true;
    const { result } = renderHook(() => useTabMaximizeOnDoubleClick(makeApi("group-a")));
    result.current(makeEvent());
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
    expect(mockExitMaximizedLayout).not.toHaveBeenCalled();
  });

  it("no-ops when api.group is missing", () => {
    const { result } = renderHook(() => useTabMaximizeOnDoubleClick(makeApi(undefined)));
    result.current(makeEvent());
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
  });

  it("reads groupId fresh at call time, not from render-time closure", () => {
    const api = makeApi("group-a");
    const { result } = renderHook(() => useTabMaximizeOnDoubleClick(api));
    // Simulate dockview reassigning the panel's group after layout rebuild.
    (api.group as { id: string }).id = "group-b";
    result.current(makeEvent());
    expect(mockMaximizeGroup).toHaveBeenCalledWith("group-b");
  });

  it("stops the event from propagating to the underlying tab", () => {
    const event = makeEvent();
    const { result } = renderHook(() => useTabMaximizeOnDoubleClick(makeApi("group-a")));
    result.current(event);
    expect(event.stopPropagation).toHaveBeenCalled();
    expect(event.preventDefault).toHaveBeenCalled();
  });
});

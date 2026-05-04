import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const SIDEBAR_GROUP_ID = "sidebar-group";

const mockMaximizeGroup = vi.fn();
const mockExitMaximizedLayout = vi.fn();

let mockPreMaximizeLayout: object | null = null;
let mockSidebarGroupId = SIDEBAR_GROUP_ID;

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      preMaximizeLayout: mockPreMaximizeLayout,
      sidebarGroupId: mockSidebarGroupId,
      maximizeGroup: mockMaximizeGroup,
      exitMaximizedLayout: mockExitMaximizedLayout,
    }),
}));

import { useToggleGroupMaximize } from "../use-tab-maximize";

describe("useToggleGroupMaximize", () => {
  beforeEach(() => {
    mockMaximizeGroup.mockClear();
    mockExitMaximizedLayout.mockClear();
    mockPreMaximizeLayout = null;
    mockSidebarGroupId = SIDEBAR_GROUP_ID;
  });

  it("calls maximizeGroup when not currently maximized", () => {
    const { result } = renderHook(() => useToggleGroupMaximize("group-a"));
    result.current();
    expect(mockMaximizeGroup).toHaveBeenCalledWith("group-a");
    expect(mockExitMaximizedLayout).not.toHaveBeenCalled();
  });

  it("calls exitMaximizedLayout when currently maximized", () => {
    mockPreMaximizeLayout = { columns: [] };
    const { result } = renderHook(() => useToggleGroupMaximize("group-a"));
    result.current();
    expect(mockExitMaximizedLayout).toHaveBeenCalledTimes(1);
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
  });

  it("no-ops when groupId is the sidebar group", () => {
    const { result } = renderHook(() => useToggleGroupMaximize(SIDEBAR_GROUP_ID));
    result.current();
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
    expect(mockExitMaximizedLayout).not.toHaveBeenCalled();
  });

  it("no-ops on sidebar group even when maximized state is set", () => {
    mockPreMaximizeLayout = { columns: [] };
    const { result } = renderHook(() => useToggleGroupMaximize(SIDEBAR_GROUP_ID));
    result.current();
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
    expect(mockExitMaximizedLayout).not.toHaveBeenCalled();
  });
});

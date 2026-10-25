import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  breakpoint: {
    isMobile: false,
    isTablet: false,
    usesDesktopWorkbench: true,
  },
  dock: {
    api: null as object | null,
    rightPanelsVisible: true,
    rightPaneVisible: true,
    rightPaneAvailable: true,
    isRestoringLayout: false,
    preMaximizeLayout: null as object | null,
    toggleRightPanels: vi.fn(),
  },
  layout: {
    columnsBySessionId: {} as Record<string, { right: boolean }>,
    toggleRightPanel: vi.fn(),
  },
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.breakpoint,
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: typeof mocks.dock) => unknown) => selector(mocks.dock),
}));

vi.mock("@/lib/state/layout-store", () => ({
  useLayoutStore: (selector: (state: typeof mocks.layout) => unknown) => selector(mocks.layout),
}));

import { useTaskRightPanelsToggle } from "./use-task-right-panels-toggle";

function setDesktop(overrides: Partial<typeof mocks.dock> = {}) {
  mocks.breakpoint = {
    isMobile: false,
    isTablet: false,
    usesDesktopWorkbench: true,
  };
  Object.assign(mocks.dock, {
    api: {},
    rightPanelsVisible: true,
    rightPaneVisible: true,
    rightPaneAvailable: true,
    isRestoringLayout: false,
    preMaximizeLayout: null,
    ...overrides,
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.breakpoint = {
    isMobile: false,
    isTablet: false,
    usesDesktopWorkbench: true,
  };
  mocks.dock.api = null;
  mocks.dock.rightPanelsVisible = true;
  mocks.dock.rightPaneVisible = true;
  mocks.dock.rightPaneAvailable = true;
  mocks.dock.isRestoringLayout = false;
  mocks.dock.preMaximizeLayout = null;
  mocks.layout.columnsBySessionId = {};
});

describe("useTaskRightPanelsToggle", () => {
  it("waits for the desktop API and settled restoration before enabling", () => {
    const { result, rerender } = renderHook(() => useTaskRightPanelsToggle("session-1"));

    expect(result.current.isSupported).toBe(true);
    expect(result.current.isReady).toBe(false);

    setDesktop({ isRestoringLayout: true });
    rerender();
    expect(result.current.isReady).toBe(false);

    mocks.dock.isRestoringLayout = false;
    rerender();
    expect(result.current.isReady).toBe(true);

    act(() => result.current.toggleRightPanels());
    expect(mocks.dock.toggleRightPanels).toHaveBeenCalledTimes(1);
    expect(mocks.layout.toggleRightPanel).not.toHaveBeenCalled();
  });

  it("routes a coarse-pointer tablet toggle to the effective session layout", () => {
    mocks.breakpoint = {
      isMobile: false,
      isTablet: true,
      usesDesktopWorkbench: false,
    };
    mocks.dock.api = {};
    mocks.dock.rightPanelsVisible = true;
    mocks.layout.columnsBySessionId = {
      "session-1": { right: false },
    };

    const { result } = renderHook(() => useTaskRightPanelsToggle("session-1"));

    expect(result.current.isSupported).toBe(true);
    expect(result.current.isReady).toBe(true);
    expect(result.current.rightPanelsVisible).toBe(false);

    act(() => result.current.toggleRightPanels());
    expect(mocks.layout.toggleRightPanel).toHaveBeenCalledWith("session-1");
    expect(mocks.dock.toggleRightPanels).not.toHaveBeenCalled();
  });

  it("disables the desktop toggle while a group is maximized", () => {
    setDesktop({ preMaximizeLayout: { columns: [] } });

    const { result } = renderHook(() => useTaskRightPanelsToggle("session-1"));

    expect(result.current.isMaximized).toBe(true);
    expect(result.current.isReady).toBe(false);
    act(() => result.current.toggleRightPanels());
    expect(mocks.dock.toggleRightPanels).not.toHaveBeenCalled();
  });

  it("does not write an empty tablet session key", () => {
    mocks.breakpoint = {
      isMobile: false,
      isTablet: true,
      usesDesktopWorkbench: false,
    };

    const { result } = renderHook(() => useTaskRightPanelsToggle(null));

    expect(result.current.isSupported).toBe(true);
    expect(result.current.isReady).toBe(false);
    act(() => result.current.toggleRightPanels());
    expect(mocks.layout.toggleRightPanel).not.toHaveBeenCalled();
  });

  it("disables the desktop toggle when the live layout has no separate right pane", () => {
    setDesktop({ rightPaneAvailable: false });

    const { result } = renderHook(() => useTaskRightPanelsToggle("session-1"));

    expect(result.current.isAvailable).toBe(false);
    expect(result.current.isReady).toBe(true);
    act(() => result.current.toggleRightPanels());
    expect(mocks.dock.toggleRightPanels).not.toHaveBeenCalled();
  });

  it("does not expose or mutate wider layouts on phones", () => {
    mocks.breakpoint = {
      isMobile: true,
      isTablet: false,
      usesDesktopWorkbench: false,
    };
    mocks.dock.api = {};
    mocks.layout.columnsBySessionId = { "session-1": { right: false } };

    const { result } = renderHook(() => useTaskRightPanelsToggle("session-1"));

    expect(result.current.isSupported).toBe(false);
    expect(result.current.isReady).toBe(false);
    act(() => result.current.toggleRightPanels());
    expect(mocks.dock.toggleRightPanels).not.toHaveBeenCalled();
    expect(mocks.layout.toggleRightPanel).not.toHaveBeenCalled();
  });
});

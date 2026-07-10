import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { usePanelActions } from "./use-panel-actions";

const responsiveState = vi.hoisted(() => ({
  value: { usesDesktopWorkbench: true },
}));

const dockActions = vi.hoisted(() => ({
  addBrowserPanel: vi.fn(),
  addPlanPanel: vi.fn(),
  addChatPanel: vi.fn(),
  addChangesPanel: vi.fn(),
  addTerminalPanel: vi.fn(),
  addVscodePanel: vi.fn(),
}));

const layoutActions = vi.hoisted(() => ({
  openDocument: vi.fn(),
  openPreview: vi.fn(),
}));

const appState = vi.hoisted(() => ({
  value: {
    tasks: {
      activeSessionId: "session-1",
      activeTaskId: "task-1",
    },
    setActiveDocument: vi.fn(),
    setPlanMode: vi.fn(),
  },
}));

const editorActions = vi.hoisted(() => ({
  openFile: vi.fn(),
  openFileInMarkdownPreview: vi.fn(),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsiveState.value,
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: typeof dockActions) => unknown) => selector(dockActions),
}));

vi.mock("@/lib/state/layout-store", () => {
  const useLayoutStore = (selector: (state: typeof layoutActions) => unknown) =>
    selector(layoutActions);
  useLayoutStore.getState = () => layoutActions;
  return { useLayoutStore };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appState.value) => unknown) => selector(appState.value),
}));

vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => editorActions,
}));

describe("usePanelActions", () => {
  beforeEach(() => {
    responsiveState.value = { usesDesktopWorkbench: true };
    vi.clearAllMocks();
  });

  it("routes compact desktop panel actions through Dockview", () => {
    const { result } = renderHook(() => usePanelActions());

    act(() => {
      result.current.addPlan();
      result.current.openFile("README.md");
    });

    expect(dockActions.addPlanPanel).toHaveBeenCalledOnce();
    expect(editorActions.openFile).toHaveBeenCalledWith("README.md", undefined);
    expect(layoutActions.openDocument).not.toHaveBeenCalled();
  });

  it("forwards the repo subpath when opening a multi-repo file", () => {
    const { result } = renderHook(() => usePanelActions());

    act(() => {
      result.current.openFile("src/foo.ts", "enrichment-commons");
    });

    expect(editorActions.openFile).toHaveBeenCalledWith("src/foo.ts", "enrichment-commons");
  });

  it("keeps non-workbench tablet fallbacks on the legacy layout store", () => {
    responsiveState.value = { usesDesktopWorkbench: false };
    const { result } = renderHook(() => usePanelActions());

    act(() => {
      result.current.addBrowser();
      result.current.addPlan();
    });

    expect(dockActions.addBrowserPanel).not.toHaveBeenCalled();
    expect(dockActions.addPlanPanel).not.toHaveBeenCalled();
    expect(layoutActions.openPreview).toHaveBeenCalledWith("session-1");
    expect(layoutActions.openDocument).toHaveBeenCalledWith("session-1");
    expect(appState.value.setPlanMode).toHaveBeenCalledWith("session-1", true);
  });
});

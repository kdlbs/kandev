import { renderHook, act } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSidebarTaskPrefs } from "./use-sidebar-task-prefs";

const mocks = vi.hoisted(() => ({
  state: {
    sidebarTaskPrefs: {
      pinnedTaskIds: [],
      orderedTaskIds: [],
      subtaskOrderByParentId: { parent: ["a", "off-page", "b"] },
    },
    togglePinnedTask: vi.fn(),
    setSidebarTaskOrder: vi.fn(),
    setSubtaskOrder: vi.fn(),
    updateSidebarDraft: vi.fn(),
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
  useAppStoreApi: () => ({ getState: () => mocks.state }),
}));
vi.mock("@/lib/state/slices/ui/sidebar-workspace-state", () => ({
  selectSidebarViews: () => ({ views: [], activeViewId: null, draft: null }),
}));

describe("useSidebarTaskPrefs", () => {
  it("merges reordered visible siblings into the stored off-page sibling order", () => {
    const { result } = renderHook(() => useSidebarTaskPrefs());

    act(() => result.current.handleReorderSubtasks("parent", ["b", "a"]));

    expect(mocks.state.setSubtaskOrder).toHaveBeenCalledWith("parent", ["b", "off-page", "a"]);
  });
});

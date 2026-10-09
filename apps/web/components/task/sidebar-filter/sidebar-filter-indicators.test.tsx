import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import type { SidebarSliceState } from "@/lib/state/slices/ui/sidebar-view-types";
import { SidebarFilterIndicators } from "./sidebar-filter-indicators";

const ACTIVE_LABEL = "Filters active";
const DRAFT_LABEL = "Unsaved filter changes";
const state = {
  workspaces: { activeId: "ws" },
  sidebarViewsByWorkspace: {} as Record<string, SidebarSliceState>,
  sidebarViews: {
    views: [DEFAULT_VIEW],
    activeViewId: DEFAULT_VIEW.id,
    draft: null,
    syncError: null,
  } as SidebarSliceState,
};
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));
const filter = { id: "review", dimension: "state", op: "is", value: "REVIEW" } as const;

describe("Sidebar filter indicators", () => {
  beforeEach(() => {
    state.sidebarViews = {
      views: [DEFAULT_VIEW],
      activeViewId: DEFAULT_VIEW.id,
      draft: null,
      syncError: null,
    };
    state.sidebarViewsByWorkspace.ws = state.sidebarViews;
  });
  afterEach(cleanup);

  it("marks a saved filtered view without claiming unsaved changes", () => {
    state.sidebarViews.views = [{ ...DEFAULT_VIEW, filters: [filter] }];
    render(<SidebarFilterIndicators />);
    expect(screen.getByRole("img", { name: ACTIVE_LABEL })).toBeTruthy();
    expect(screen.queryByRole("img", { name: DRAFT_LABEL })).toBeNull();
  });

  it("distinguishes a sorting draft from active filters", () => {
    state.sidebarViews.draft = {
      ...DEFAULT_VIEW,
      baseViewId: DEFAULT_VIEW.id,
      sort: { key: "title", direction: "asc" },
    };
    render(<SidebarFilterIndicators />);
    expect(screen.queryByRole("img", { name: ACTIVE_LABEL })).toBeNull();
    expect(screen.getByRole("img", { name: DRAFT_LABEL })).toBeTruthy();
  });

  it("uses draft filters immediately, including removing the saved filter", () => {
    state.sidebarViews.views = [{ ...DEFAULT_VIEW, filters: [filter] }];
    state.sidebarViews.draft = { ...DEFAULT_VIEW, baseViewId: DEFAULT_VIEW.id };
    render(<SidebarFilterIndicators />);
    expect(screen.queryByRole("img", { name: ACTIVE_LABEL })).toBeNull();
    expect(screen.getByRole("img", { name: DRAFT_LABEL })).toBeTruthy();
  });

  it("ignores a draft belonging to another view", () => {
    state.sidebarViews.draft = { ...DEFAULT_VIEW, baseViewId: "other", filters: [filter] };
    render(<SidebarFilterIndicators />);
    expect(screen.queryByRole("img", { name: ACTIVE_LABEL })).toBeNull();
    expect(screen.queryByRole("img", { name: DRAFT_LABEL })).toBeNull();
  });
});

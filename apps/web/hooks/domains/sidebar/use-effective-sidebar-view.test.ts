import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { DEFAULT_VIEW } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { useEffectiveSidebarView } from "./use-effective-sidebar-view";

const mocks = vi.hoisted(() => ({
  state: {
    workspaces: { activeId: null as string | null },
    sidebarViewsByWorkspace: {},
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

describe("useEffectiveSidebarView", () => {
  it("returns the default view while no workspace view is available", () => {
    const { result } = renderHook(() => useEffectiveSidebarView(null));

    expect(result.current).toEqual(DEFAULT_VIEW);
  });
});

it("starts a different draft grouping expanded without changing the saved disclosure", () => {
  const source = {
    ...DEFAULT_VIEW,
    id: "saved",
    group: "plugin:tags:tags" as const,
    collapsedGroups: ["retained"],
  };
  mocks.state.sidebarViewsByWorkspace = {
    ws: {
      views: [source],
      activeViewId: source.id,
      syncError: null,
      draft: {
        baseViewId: source.id,
        filters: source.filters,
        sort: source.sort,
        group: "plugin:tags:multi-tags",
        groupIndent: true,
      },
    },
  };
  const { result } = renderHook(() => useEffectiveSidebarView("ws"));
  expect(result.current.collapsedGroups).toEqual([]);
  expect(source.collapsedGroups).toEqual(["retained"]);
});

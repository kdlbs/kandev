import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";

const ALL_VIEW: SidebarView = {
  id: "view-all",
  name: "All tasks",
  filters: [],
  sort: { key: "state", direction: "asc" },
  group: "repository",
  collapsedGroups: [],
};

const mockState = vi.hoisted(() => ({
  workspaces: { activeId: "ws" as string | null },
  sidebarViewsByWorkspace: {
    ws: {
      views: [] as SidebarView[],
      activeViewId: "view-all",
      draft: null as { baseViewId: string } | null,
      syncError: null,
    },
  },
  createSidebarView: vi.fn<() => string | null>(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => mockState }),
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
}));

import { getNewViewDisabledReason, useSidebarViewPopover } from "./use-sidebar-view-popover";

beforeEach(() => {
  mockState.workspaces.activeId = "ws";
  mockState.sidebarViewsByWorkspace.ws.views = [ALL_VIEW];
  mockState.sidebarViewsByWorkspace.ws.activeViewId = ALL_VIEW.id;
  mockState.sidebarViewsByWorkspace.ws.draft = null;
  mockState.createSidebarView.mockReset();
  mockState.createSidebarView.mockImplementation(() => {
    const created = { ...ALL_VIEW, id: "view-new", name: "New view" };
    mockState.sidebarViewsByWorkspace.ws.views = [
      ...mockState.sidebarViewsByWorkspace.ws.views,
      created,
    ];
    mockState.sidebarViewsByWorkspace.ws.activeViewId = created.id;
    return created.id;
  });
});

afterEach(cleanup);

describe("useSidebarViewPopover", () => {
  it("opens rename for the exact view returned by instant creation", () => {
    const { result } = renderHook(() => useSidebarViewPopover());

    act(() => expect(result.current.startNewView()).toBe(true));

    expect(result.current.open).toBe(true);
    expect(result.current.renameRequestedViewId).toBe("view-new");
    expect(mockState.createSidebarView).toHaveBeenCalledOnce();

    act(() => result.current.consumeRenameRequest("view-new"));
    expect(result.current.renameRequestedViewId).toBeNull();
  });

  it("clears pending rename when the popover closes or creation rolls back", async () => {
    const { result, rerender } = renderHook(() => useSidebarViewPopover());
    act(() => expect(result.current.startNewView()).toBe(true));

    act(() => result.current.onOpenChange(false));
    expect(result.current.renameRequestedViewId).toBeNull();

    act(() => expect(result.current.startNewView()).toBe(true));
    mockState.sidebarViewsByWorkspace.ws.views = [ALL_VIEW];
    rerender();
    await waitFor(() => expect(result.current.renameRequestedViewId).toBeNull());
  });

  it("blocks creation for a draft or the 50-view limit", () => {
    expect(getNewViewDisabledReason(1, true)).toMatch(/save or discard/i);
    expect(getNewViewDisabledReason(50, false)).toMatch(/50/);

    mockState.sidebarViewsByWorkspace.ws.draft = { baseViewId: ALL_VIEW.id };
    const { result, rerender } = renderHook(() => useSidebarViewPopover());
    expect(result.current.newViewDisabledReason).toMatch(/save or discard/i);
    act(() => expect(result.current.startNewView()).toBe(false));
    expect(mockState.createSidebarView).not.toHaveBeenCalled();

    mockState.sidebarViewsByWorkspace.ws.draft = null;
    mockState.sidebarViewsByWorkspace.ws.views = Array.from({ length: 50 }, (_, index) => ({
      ...ALL_VIEW,
      id: `view-${index}`,
      name: `View ${index}`,
    }));
    rerender();
    expect(result.current.newViewDisabledReason).toMatch(/50/);
    act(() => expect(result.current.startNewView()).toBe(false));
    expect(mockState.createSidebarView).not.toHaveBeenCalled();
  });
});

it("dismisses an editor when workspace changes even with identical view IDs", () => {
  const { result, rerender } = renderHook(() => useSidebarViewPopover());
  act(() => result.current.onOpenChange(true));
  expect(result.current.open).toBe(true);
  mockState.workspaces.activeId = "other";
  rerender();
  expect(result.current.open).toBe(false);
});

it("ignores a create callback retained from a previous workspace", () => {
  const { result } = renderHook(() => useSidebarViewPopover());
  const stale = result.current.startNewView;
  mockState.workspaces.activeId = "other";
  act(() => expect(stale()).toBe(false));
  expect(mockState.createSidebarView).not.toHaveBeenCalled();
});

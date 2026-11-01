import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { applyGroup, flattenVisibleTaskIds } from "@/lib/sidebar/apply-view";
import { shareGroupedSidebarList } from "@/components/task/task-session-sidebar-grouped-view";
import { useSidebarFacetGrouping } from "./use-sidebar-facet-grouping";

const PLUGIN = "sidebar-facet-test";
const FACET = "facet:sidebar-facet-test:tags";
const parent = { id: "parent", title: "Parent", state: "TODO" as const };
const child = { id: "child", title: "Child", state: "TODO" as const, parentTaskId: "parent" };
const page = applyGroup([parent, child], "none");

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN);
});

describe("useSidebarFacetGrouping", () => {
  it("retains color-only live updates through grouped-list memoization", () => {
    let color = "#123456";
    let notify = () => undefined as void;
    pluginRegistry.forPlugin(PLUGIN).registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => [{ value: "alpha", label: "Alpha", color }],
      subscribe: (listener) => {
        notify = listener;
        return () => undefined;
      },
    });
    const { result } = renderHook(() => useSidebarFacetGrouping(page, FACET, "ws-1"));
    const before = result.current;
    color = "#abcdef";
    act(() => notify());
    const after = shareGroupedSidebarList(before, result.current);
    expect(after.groups[0].color).toBe("#abcdef");
    expect(after.groups[0]).not.toBe(before.groups[0]);
  });

  it("temporarily falls back without changing the request and resumes on registration", () => {
    const { result } = renderHook(() => useSidebarFacetGrouping(page, FACET, "ws-1"));
    expect(result.current).toBe(page);
    act(() => {
      pluginRegistry.forPlugin(PLUGIN).registerTaskListFacet({
        id: "tags",
        label: "Tag",
        getValues: () => [{ value: "alpha", label: "Alpha" }],
      });
    });
    expect(result.current.groupKey).toBe(FACET);
    expect(result.current.groups.map((group) => group.label)).toEqual(["Alpha"]);
    act(() => pluginRegistry.unregisterPlugin(PLUGIN));
    expect(result.current).toBe(page);
  });

  it("updates membership and labels live, retains per-section children through sharing and cleans up", () => {
    let shared = true;
    let label = "Alpha";
    let notify = () => undefined as void;
    const unsubscribe = vi.fn();
    pluginRegistry.forPlugin(PLUGIN).registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: ({ taskId }) => [
        {
          value: taskId === "child" && !shared ? "beta" : "alpha",
          label: taskId === "child" && !shared ? "Beta" : label,
        },
      ],
      subscribe: (listener) => {
        notify = listener;
        return unsubscribe;
      },
    });
    const { result, unmount } = renderHook(() => useSidebarFacetGrouping(page, FACET, "ws-1"));
    const before = result.current;
    expect(before.groups[0].subTasksByParentId?.get("parent")).toEqual([child]);
    shared = false;
    act(() => notify());
    const after = shareGroupedSidebarList(before, result.current);
    expect(after.groups.map((group) => group.label)).toEqual(["Alpha", "Beta"]);
    expect(after.groups[0].subTasksByParentId?.get("parent")).toBeUndefined();
    expect(flattenVisibleTaskIds(after, [], [])).toEqual(["parent", "child"]);
    label = "Renamed";
    act(() => notify());
    expect(result.current.groups.map((group) => group.label)).toEqual(["Beta", "Renamed"]);
    unmount();
    expect(unsubscribe).toHaveBeenCalledOnce();
  });

  it("passes the explicit workspace and reprojects when it changes, with no cross-workspace fallback", () => {
    const getValues = vi.fn(({ workspaceId }: { workspaceId?: string }) => [
      { value: workspaceId!, label: workspaceId! },
    ]);
    pluginRegistry.forPlugin(PLUGIN).registerTaskListFacet({ id: "tags", label: "Tag", getValues });
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useSidebarFacetGrouping(page, FACET, workspaceId),
      {
        initialProps: { workspaceId: "ws-1" as string | null },
      },
    );
    expect(result.current.groups[0].label).toBe("ws-1");
    rerender({ workspaceId: "ws-2" });
    expect(result.current.groups[0].label).toBe("ws-2");
    expect(getValues).toHaveBeenCalledWith({ taskId: "parent", workspaceId: "ws-2" });
    rerender({ workspaceId: null });
    expect(result.current).toBe(page);
  });

  it("does not change builtin grouping", () => {
    const builtin = applyGroup([parent], "state");
    const { result } = renderHook(() => useSidebarFacetGrouping(builtin, "state", "ws-1"));
    expect(result.current).toBe(builtin);
  });
});

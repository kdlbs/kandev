import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  resetPluginTaskFilterSelectionsForTests,
  usePluginTaskFilters,
} from "./use-plugin-task-filters";

const PLUGIN_ID = "kandev-plugin-tags";

describe("usePluginTaskFilters", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
    resetPluginTaskFilterSelectionsForTests();
  });

  it("returns registered task filters", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskFilter({
      id: "tags",
      label: "Tags",
      getOptions: () => [{ value: "bug", label: "Bug" }],
      matches: () => true,
    });

    const { result } = renderHook(() => usePluginTaskFilters());

    expect(result.current.filters).toHaveLength(1);
    expect(result.current.filters[0]).toMatchObject({ id: "tags", pluginId: PLUGIN_ID });
  });

  it("matches every task when no selection is set (implicit All)", () => {
    const matches = vi.fn().mockReturnValue(false);
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskFilter({
      id: "tags",
      label: "Tags",
      getOptions: () => [],
      matches,
    });

    const { result } = renderHook(() => usePluginTaskFilters());

    expect(result.current.taskMatchesPluginFilters({ taskId: "task-1" })).toBe(true);
    expect(matches).not.toHaveBeenCalled();
  });

  it("delegates to matches() once a selection is set, and clearing restores All", () => {
    const matches = vi.fn((_context: { taskId: string }, selected: string[]) =>
      selected.includes("bug"),
    );
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskFilter({
      id: "tags",
      label: "Tags",
      getOptions: () => [
        { value: "bug", label: "Bug" },
        { value: "feature", label: "Feature" },
      ],
      matches,
    });

    const { result } = renderHook(() => usePluginTaskFilters());

    act(() => result.current.setFilterSelection("tags", ["feature"]));
    expect(result.current.taskMatchesPluginFilters({ taskId: "task-1" })).toBe(false);
    expect(matches).toHaveBeenCalledWith({ taskId: "task-1" }, ["feature"]);

    act(() => result.current.setFilterSelection("tags", []));
    expect(result.current.taskMatchesPluginFilters({ taskId: "task-1" })).toBe(true);
  });

  it("catches a throwing matches() and treats it as non-matching", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskFilter({
      id: "tags",
      label: "Tags",
      getOptions: () => [{ value: "bug", label: "Bug" }],
      matches: () => {
        throw new Error("boom");
      },
    });

    const { result } = renderHook(() => usePluginTaskFilters());
    act(() => result.current.setFilterSelection("tags", ["bug"]));

    expect(result.current.taskMatchesPluginFilters({ taskId: "task-1" })).toBe(false);
    errorSpy.mockRestore();
  });

  it("requires every registered filter to match (AND semantics across filters)", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskFilter({
      id: "tags",
      label: "Tags",
      getOptions: () => [{ value: "bug", label: "Bug" }],
      matches: () => true,
    });
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskFilter({
      id: "priority",
      label: "Priority",
      getOptions: () => [{ value: "high", label: "High" }],
      matches: () => false,
    });

    const { result } = renderHook(() => usePluginTaskFilters());
    act(() => result.current.setFilterSelection("tags", ["bug"]));
    act(() => result.current.setFilterSelection("priority", ["high"]));

    expect(result.current.taskMatchesPluginFilters({ taskId: "task-1" })).toBe(false);
  });

  it("shares selection state across independent hook instances (singleton store)", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskFilter({
      id: "tags",
      label: "Tags",
      getOptions: () => [{ value: "bug", label: "Bug" }],
      matches: (_context, selected) => selected.includes("bug"),
    });

    const dropdown = renderHook(() => usePluginTaskFilters());
    const board = renderHook(() => usePluginTaskFilters());

    act(() => dropdown.result.current.setFilterSelection("tags", ["bug"]));

    expect(board.result.current.selections).toEqual({ tags: ["bug"] });
    expect(board.result.current.taskMatchesPluginFilters({ taskId: "task-1" })).toBe(true);
  });
});

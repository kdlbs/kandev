import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ResolvedTaskListFacet } from "./use-task-list-facets";
import { resolveTaskFacetValues, useTaskListFacets } from "./use-task-list-facets";
import { pluginRegistry } from "@/lib/plugins/registry";

const TASKS = [{ id: "task-1" }] as never[];
const PLUGIN_ID = "facet-hook-test";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  vi.restoreAllMocks();
});

function facet(overrides: Partial<ResolvedTaskListFacet> = {}): ResolvedTaskListFacet {
  return {
    pluginId: PLUGIN_ID,
    id: "tags",
    label: "Tag",
    key: `facet:${PLUGIN_ID}:tags`,
    getValues: () => [],
    ...overrides,
  };
}

describe("resolveTaskFacetValues", () => {
  it("returns no values without a workspace", () => {
    expect(resolveTaskFacetValues([facet()], TASKS, null)).toEqual({});
  });

  it("filters malformed values and contains callback failures", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const values = resolveTaskFacetValues(
      [
        facet({
          getValues: () => [
            { value: "valid", label: "Valid" },
            { value: "missing-label" } as never,
            null as never,
          ],
        }),
        facet({
          id: "broken",
          key: "facet:facet-hook-test:broken",
          getValues: () => {
            throw new Error("nope");
          },
        }),
      ],
      TASKS,
      "workspace-1",
    );

    expect(values).toEqual({
      [`facet:${PLUGIN_ID}:tags:task-1`]: [{ value: "valid", label: "Valid" }],
      [`facet:${PLUGIN_ID}:broken:task-1`]: [],
    });
  });
});

describe("useTaskListFacets", () => {
  it("keeps healthy subscriptions active when another facet throws and cleans them up", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const unsubscribe = vi.fn();
    const subscribe = vi.fn(() => unsubscribe);
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskListFacet({
      id: "healthy",
      label: "Healthy",
      getValues: () => [],
      subscribe,
    });
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskListFacet({
      id: "broken",
      label: "Broken",
      getValues: () => [],
      subscribe: () => {
        throw new Error("nope");
      },
    });

    const { unmount } = renderHook(() => useTaskListFacets(TASKS, "workspace-1"));

    expect(subscribe).toHaveBeenCalledOnce();
    act(unmount);
    expect(unsubscribe).toHaveBeenCalledOnce();
  });
});

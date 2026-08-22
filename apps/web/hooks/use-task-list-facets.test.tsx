import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ResolvedTaskListFacet } from "./use-task-list-facets";
import { resolveTaskFacetValues, useTaskListFacets } from "./use-task-list-facets";
import { pluginRegistry } from "@/lib/plugins/registry";

const TASKS = [{ id: "task-1" }] as never[];
const PLUGIN_ID = "facet-hook-test";
const WORKSPACE_ID = "workspace-1";

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
      WORKSPACE_ID,
    );

    expect(values).toEqual({
      [`facet:${PLUGIN_ID}:tags:task-1`]: [{ value: "valid", label: "Valid" }],
      [`facet:${PLUGIN_ID}:broken:task-1`]: [],
    });
  });
});

describe("useTaskListFacets", () => {
  it("namespaces each facet key with its plugin and registration id", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => [],
    });

    const { result } = renderHook(() => useTaskListFacets(TASKS, WORKSPACE_ID));

    expect(result.current.facets.map((entry) => entry.key)).toEqual([`facet:${PLUGIN_ID}:tags`]);
  });

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

    const { unmount } = renderHook(() => useTaskListFacets(TASKS, WORKSPACE_ID));

    expect(subscribe).toHaveBeenCalledOnce();
    act(unmount);
    expect(unsubscribe).toHaveBeenCalledOnce();
  });

  // The facet is a live projection, not a load-time snapshot: a plugin whose
  // catalog changes after mount notifies through subscribe(), and every consumer
  // of `values` (facet sort order, facet group sections) has to see the new
  // labels without the task list itself refetching.
  it("reprojects values when a facet notifies its subscriber", () => {
    let labels = ["Alpha"];
    let notify = () => undefined as void;
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => labels.map((label) => ({ value: label.toLowerCase(), label })),
      subscribe: (listener) => {
        notify = listener;
        return () => undefined;
      },
    });

    const { result } = renderHook(() => useTaskListFacets(TASKS, WORKSPACE_ID));
    const key = `facet:${PLUGIN_ID}:tags:task-1`;
    expect(result.current.values[key]).toEqual([{ value: "alpha", label: "Alpha" }]);

    labels = ["Beta", "Gamma"];
    act(() => notify());

    expect(result.current.values[key]).toEqual([
      { value: "beta", label: "Beta" },
      { value: "gamma", label: "Gamma" },
    ]);
  });

  it("passes the active workspace id to getValues", () => {
    const getValues = vi.fn(() => []);
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues,
    });

    renderHook(() => useTaskListFacets(TASKS, WORKSPACE_ID));

    expect(getValues).toHaveBeenCalledWith({ taskId: "task-1", workspaceId: WORKSPACE_ID });
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "./registry";

const pluginId = "plugin-a";

describe("pluginRegistry task-list facet callbacks", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(pluginId);
  });

  it("accepts a context-only facet and notifies consumers when its owner unloads", () => {
    const listener = vi.fn();
    const unsubscribe = pluginRegistry.subscribe(listener);
    const getValues = ({ taskId, workspaceId }: { taskId: string; workspaceId?: string }) => [
      { value: `${workspaceId}:${taskId}`, label: "Tag" },
    ];
    const subscribe = () => () => {};
    try {
      pluginRegistry.forPlugin(pluginId).registerTaskListFacet({
        id: "tags",
        label: "Tag",
        getValues,
        subscribe,
      });
      expect(
        pluginRegistry.getTaskListFacets()[0]?.getValues({
          taskId: "task",
          workspaceId: "workspace",
        }),
      ).toEqual([{ value: "workspace:task", label: "Tag" }]);
      listener.mockClear();
      pluginRegistry.unregisterPlugin(pluginId);
      expect(pluginRegistry.getTaskListFacets()).toEqual([]);
      expect(listener).toHaveBeenCalledTimes(1);
    } finally {
      unsubscribe();
    }
  });
});

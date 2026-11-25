import { afterEach, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  sidebarTaskPageScope,
  sidebarTaskPageRankingKey,
} from "@/lib/sidebar/sidebar-task-page-cache";
import { createSidebarRequestCheck } from "./sidebar-task-page-load";

afterEach(() => pluginRegistry.setSidebarGroupings([]));

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
it("fences a plugin source update synchronously, without invalidating builtin or foreign-workspace reads", () => {
  const store = createAppStore();
  store.setState({ workspaces: { items: [], activeId: "ws" }, workspaceContextGeneration: 1 });
  const reference = "plugin:acme:labels";
  pluginRegistry.setSidebarGroupings([{ reference, label: "Labels", generation: "generation-1" }]);
  const args = {
    store,
    workspaceId: "ws",
    generationRef: { current: 1 },
    generation: 1,
    key: "view",
    viewKeyRef: { current: "view" },
    workspaceGeneration: 1,
    scope: sidebarTaskPageScope(store.getState()),
    rankingKey: sidebarTaskPageRankingKey(store.getState()),
  };
  const check = createSidebarRequestCheck({ ...args, groupingReference: reference });
  const builtin = createSidebarRequestCheck({ ...args, groupingReference: "state" });
  pluginRegistry.invalidateSidebarGroupings("acme", "another-workspace");
  expect(check()).toBe(true);
  pluginRegistry.invalidateSidebarGroupings("another-plugin", "ws");
  expect(check()).toBe(true);
  pluginRegistry.invalidateSidebarGroupings("acme", "ws");
  expect(check()).toBe(false);
  expect(builtin()).toBe(true);
});

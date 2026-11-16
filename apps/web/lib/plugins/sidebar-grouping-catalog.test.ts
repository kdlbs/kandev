import { afterEach, expect, it, vi } from "vitest";
import type { SidebarGroupingProvider } from "@kdlbs/plugin-sdk";
import { createAppStore } from "@/lib/state/store";
import { listSidebarGroupingProviders } from "@/lib/api/domains/plugins-api";
import { pluginRegistry } from "./registry";
import { refreshSidebarGroupingCatalog } from "./sidebar-grouping-catalog";

vi.mock("@/lib/api/domains/plugins-api", () => ({ listSidebarGroupingProviders: vi.fn() }));
const provider: SidebarGroupingProvider = {
  reference: "plugin:acme:labels",
  label: "Labels",
  generation: "old",
};
afterEach(() => {
  vi.resetAllMocks();
  pluginRegistry.setSidebarGroupings([]);
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
it("does not restore a removed provider from an older catalog response", async () => {
  const store = createAppStore();
  const old = Promise.withResolvers<readonly SidebarGroupingProvider[]>();
  vi.mocked(listSidebarGroupingProviders)
    .mockReturnValueOnce(old.promise)
    .mockResolvedValueOnce([]);
  const pending = refreshSidebarGroupingCatalog(store);
  await refreshSidebarGroupingCatalog(store);
  old.resolve([provider]);
  await pending;
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
});

it("does not publish an old workspace or account context's catalog", async () => {
  const store = createAppStore();
  const old = Promise.withResolvers<readonly SidebarGroupingProvider[]>();
  vi.mocked(listSidebarGroupingProviders).mockReturnValueOnce(old.promise);
  const pending = refreshSidebarGroupingCatalog(store);
  store.setState((state) => ({ workspaceContextGeneration: state.workspaceContextGeneration + 1 }));
  old.resolve([provider]);
  await pending;
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
});

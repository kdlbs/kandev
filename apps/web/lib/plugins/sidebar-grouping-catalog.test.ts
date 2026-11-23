import { afterEach, expect, it, vi } from "vitest";
import type { SidebarGroupingProvider } from "@kdlbs/plugin-sdk";
import { createAppStore } from "@/lib/state/store";
import { listSidebarGroupingProviders } from "@/lib/api/domains/plugins-api";
import { ApiError } from "@/lib/api/client";
import { bootPlugins } from "./boot";
import { pluginRegistry } from "./registry";
import {
  reconcileSidebarGroupingCatalogScope,
  refreshSidebarGroupingCatalog,
} from "./sidebar-grouping-catalog";
import { registerSidebarGroupingHandlers } from "@/lib/ws/handlers/sidebar-grouping";

vi.mock("@/lib/api/domains/plugins-api", () => ({ listSidebarGroupingProviders: vi.fn() }));
vi.mock("./host", () => ({ installPluginGlobal: vi.fn(), loadPlugins: vi.fn() }));
const provider: SidebarGroupingProvider = {
  reference: "plugin:acme:labels",
  label: "Labels",
  generation: "old",
};
afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.resetAllMocks();
  pluginRegistry.setSidebarGroupings([]);
});

it("preserves other provider identities through catalog additions and replacements", () => {
  pluginRegistry.setSidebarGroupings([provider]);
  const before = pluginRegistry.sidebarGroupingIdentity(provider.reference, "workspace");
  const other: SidebarGroupingProvider = {
    reference: "plugin:other:labels",
    label: "Other",
    generation: "g1",
  };
  pluginRegistry.setSidebarGroupings([provider, other]);
  expect(pluginRegistry.sidebarGroupingIdentity(provider.reference, "workspace")).toBe(before);
  const otherBefore = pluginRegistry.sidebarGroupingIdentity(other.reference, "workspace");
  pluginRegistry.setSidebarGroupings([provider, { ...other, generation: "g2" }]);
  expect(pluginRegistry.sidebarGroupingIdentity(provider.reference, "workspace")).toBe(before);
  expect(pluginRegistry.sidebarGroupingIdentity(other.reference, "workspace")).not.toBe(
    otherBefore,
  );
  pluginRegistry.setSidebarGroupings([{ ...provider, generation: "new" }]);
  expect(pluginRegistry.sidebarGroupingIdentity(provider.reference, "workspace")).not.toBe(before);
});

it("catalog events fence only the changed provider before refreshing metadata", async () => {
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  const before = pluginRegistry.sidebarGroupingIdentity(provider.reference, "workspace");
  vi.mocked(listSidebarGroupingProviders).mockResolvedValue([provider]);
  const handler = registerSidebarGroupingHandlers(store)["plugin.sidebar-grouping.updated"]!;
  handler({
    type: "notification",
    action: "plugin.sidebar-grouping.updated",
    payload: { catalog: true, plugin_id: "other" },
  });
  await vi.waitFor(() => expect(listSidebarGroupingProviders).toHaveBeenCalledOnce());
  expect(pluginRegistry.sidebarGroupingIdentity(provider.reference, "workspace")).toBe(before);
  handler({
    type: "notification",
    action: "plugin.sidebar-grouping.updated",
    payload: { catalog: true, plugin_id: "acme" },
  });
  expect(pluginRegistry.sidebarGroupingIdentity(provider.reference, "workspace")).not.toBe(before);
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

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
it("retains eligible boot metadata during a temporary failure and recovers without reconnecting", async () => {
  vi.useFakeTimers();
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  const recovered = { ...provider, generation: "recovered" };
  vi.mocked(listSidebarGroupingProviders)
    .mockRejectedValueOnce(new ApiError("temporarily unavailable", 503, null))
    .mockResolvedValueOnce([recovered]);
  const pending = refreshSidebarGroupingCatalog(store);
  await vi.advanceTimersByTimeAsync(0);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([provider]);
  await vi.runAllTimersAsync();
  await pending;
  expect(pluginRegistry.getSidebarGroupings()).toEqual([recovered]);
});

it("recovers initial discovery after one network failure with no connection or scope change", async () => {
  vi.useFakeTimers();
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [] }, store);
  vi.mocked(listSidebarGroupingProviders)
    .mockRejectedValueOnce(new TypeError())
    .mockResolvedValueOnce([provider]);
  const pending = refreshSidebarGroupingCatalog(store);
  await vi.runAllTimersAsync();
  await pending;
  expect(pluginRegistry.getSidebarGroupings()).toEqual([provider]);
});

it("removes a retained provider only after an authoritative empty catalog", async () => {
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  vi.mocked(listSidebarGroupingProviders).mockResolvedValueOnce([]);
  await refreshSidebarGroupingCatalog(store);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
});

it("retains eligible metadata when bounded temporary recovery is exhausted", async () => {
  vi.useFakeTimers();
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  vi.mocked(listSidebarGroupingProviders).mockRejectedValue(new TypeError());
  const pending = refreshSidebarGroupingCatalog(store);
  await vi.advanceTimersByTimeAsync(5_000);
  await pending;
  expect(pluginRegistry.getSidebarGroupings()).toEqual([provider]);
  const requests = vi.mocked(listSidebarGroupingProviders).mock.calls.length;
  await vi.advanceTimersByTimeAsync(60_000);
  expect(vi.mocked(listSidebarGroupingProviders).mock.calls).toHaveLength(requests);
  expect(vi.getTimerCount()).toBe(0);
});

it("does not let a queued retry undo a newer authoritative removal", async () => {
  vi.useFakeTimers();
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  vi.mocked(listSidebarGroupingProviders)
    .mockRejectedValueOnce(new TypeError())
    .mockResolvedValueOnce([]);
  const old = refreshSidebarGroupingCatalog(store);
  await vi.advanceTimersByTimeAsync(0);
  await refreshSidebarGroupingCatalog(store);
  await vi.runAllTimersAsync();
  await old;
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
});

it("clears the old scope while a successor context is still discovering providers", async () => {
  vi.useFakeTimers();
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  const successor = Promise.withResolvers<readonly SidebarGroupingProvider[]>();
  vi.mocked(listSidebarGroupingProviders)
    .mockRejectedValueOnce(new TypeError())
    .mockReturnValueOnce(successor.promise);
  const old = refreshSidebarGroupingCatalog(store);
  await vi.advanceTimersByTimeAsync(0);
  store.setState((state) => ({ workspaceContextGeneration: state.workspaceContextGeneration + 1 }));
  const current = refreshSidebarGroupingCatalog(store);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
  await vi.runAllTimersAsync();
  await old;
  successor.resolve([{ ...provider, generation: "successor" }]);
  await current;
  expect(pluginRegistry.getSidebarGroupings()).toEqual([{ ...provider, generation: "successor" }]);
});

it.each([401, 403])("clears retained metadata on access denial (%s)", async (status) => {
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  vi.mocked(listSidebarGroupingProviders).mockRejectedValueOnce(
    new ApiError("denied", status, null),
  );
  await refreshSidebarGroupingCatalog(store);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
});

it("clears cached metadata at a disconnected scope boundary without starting a read", () => {
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([provider]);
  store.setState((state) => ({ workspaceContextGeneration: state.workspaceContextGeneration + 1 }));
  reconcileSidebarGroupingCatalogScope(store);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
  expect(listSidebarGroupingProviders).not.toHaveBeenCalled();
});

it("does not overwrite a successor store's boot metadata with an older store's response", async () => {
  const oldStore = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, oldStore);
  const response = Promise.withResolvers<readonly SidebarGroupingProvider[]>();
  vi.mocked(listSidebarGroupingProviders).mockReturnValueOnce(response.promise);
  const old = refreshSidebarGroupingCatalog(oldStore);
  const successor = { ...provider, generation: "new-store" };
  bootPlugins({ sidebarGroupings: [successor] }, createAppStore());
  response.resolve([provider]);
  await old;
  expect(pluginRegistry.getSidebarGroupings()).toEqual([successor]);
});

it("does not retry earlier than a server delay beyond the recovery budget", async () => {
  vi.useFakeTimers();
  const store = createAppStore();
  bootPlugins({ sidebarGroupings: [provider] }, store);
  vi.mocked(listSidebarGroupingProviders)
    .mockRejectedValueOnce(new ApiError("unavailable", 503, null, 30))
    .mockResolvedValueOnce([]);
  await refreshSidebarGroupingCatalog(store);
  await vi.advanceTimersByTimeAsync(30_000);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([provider]);
  expect(vi.getTimerCount()).toBe(0);
});

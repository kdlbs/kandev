import { afterEach, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";

import { bootPlugins } from "./boot";
import { pluginRegistry } from "./registry";

afterEach(() => pluginRegistry.setSidebarGroupings([]));

it("does not restore revoked providers from stale boot metadata on a repeated bridge effect", () => {
  const store = createAppStore();
  const provider = {
    reference: "plugin:acme:labels" as const,
    label: "Tag",
    generation: "install-1",
  };
  const payload = { sidebarGroupings: [provider] };
  bootPlugins(payload, store);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([provider]);
  pluginRegistry.setSidebarGroupings([]);
  bootPlugins(payload, store);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
});

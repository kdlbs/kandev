import { describe, it, expect, afterEach, vi } from "vitest";
import * as React from "react";
import { render, screen, cleanup } from "@testing-library/react";
import { loadPlugins } from "./host";
import { pluginRegistry } from "./registry";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import type { ActivePlugin, PluginHostApi, PluginRegistry } from "./types";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "" }) }));

const PLUGIN_ID = "kandev-plugin-render-dup";
const SLOT = "chat-input-actions";
const ICON_TESTID = "plugin-cost-icon";

/**
 * Host factory that hands the plugin a *real* React + jsx, exactly like the
 * production host (lib/plugins/host-api.ts) — so the plugin's registered slot
 * component renders real DOM we can see, the same way the fixture bundle
 * (apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js) draws its
 * widget through host.React / host.jsx.
 */
function makeHostFactory(pluginId: string): PluginHostApi {
  return {
    pluginId,
    React,
    jsx: React.createElement,
    store: { getState: () => ({}) as never, setState: () => {}, subscribe: () => () => {} },
    api: { fetch: async () => new Response(), baseUrl: "" },
    ui: {},
    theme: "light",
    navigate: () => {},
  };
}

/**
 * Fake dynamic import that mimics the browser ESM cache: the bundle's
 * top-level `registerKandevPlugin(...)` runs only on the *first* import of a
 * given specifier; a later import of the same URL resolves from cache without
 * re-executing it. The registered plugin draws a single icon into the slot.
 */
function makeImporter() {
  let imported = 0;
  return async (_url: string) => {
    imported += 1;
    if (imported === 1) {
      (
        window as unknown as { registerKandevPlugin: (id: string, plugin: unknown) => void }
      ).registerKandevPlugin(PLUGIN_ID, {
        initialize: (registry: PluginRegistry, host: PluginHostApi) => {
          const jsx = host.jsx;
          registry.registerComponent(SLOT, function CostIcon() {
            return jsx(
              "button",
              {
                "data-testid": ICON_TESTID,
                "aria-label": "Session cost",
                className: "cursor-pointer",
              },
              "🪙",
            );
          });
        },
      });
    }
    return {};
  };
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("chat-input-actions renders one plugin icon after a re-boot (the reported duplicate)", () => {
  it("shows exactly one icon in the real toolbar slot when the plugin is loaded twice without an explicit unload", async () => {
    const plugin: ActivePlugin = {
      id: PLUGIN_ID,
      name: "Session Cost",
      bundleUrl: `/api/plugins/${PLUGIN_ID}/bundle?v=1`,
    };
    const importer = makeImporter();

    // First boot loads the plugin (registers its chat-bar icon).
    await loadPlugins([plugin], makeHostFactory, importer);
    // A second load with no unloadPlugin() in between — the boot re-entry the
    // #1861 cache-bust fix cannot cover (boot race / dev HMR / new store). The
    // cached bundle registration is reused, so initialize() runs again.
    await loadPlugins([plugin], makeHostFactory, importer);

    render(
      <PluginSlot
        name={SLOT}
        slotProps={{ taskId: null, activeSessionId: null, sessionIds: [] }}
      />,
    );

    // Before the fix this renders two identical icons (the screenshot).
    expect(screen.getAllByTestId(ICON_TESTID)).toHaveLength(1);
  });
});

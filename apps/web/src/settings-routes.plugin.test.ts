import { isValidElement, type ReactElement } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { renderSettingsRoute } from "./settings-routes";

const PLUGIN_ID = "plugin-a";
const PLUGIN_SETTINGS_PATH = "/settings/plugins/plugin-a/config";

function cleanupPlugins(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
}

describe("renderSettingsRoute — plugin fallthrough", () => {
  afterEach(() => cleanupPlugins(PLUGIN_ID));

  it("falls back to the unported-route placeholder when no plugin owns the path", () => {
    const route = renderSettingsRoute(PLUGIN_SETTINGS_PATH);
    expect(isValidElement(route)).toBe(true);
    // The fallback renders the raw pathname as text — assert via string search
    // on the rendered props rather than a brittle component-type check.
    expect((route as ReactElement<{ pathname?: string }>).props.pathname).toBe(
      PLUGIN_SETTINGS_PATH,
    );
  });

  it("renders the plugin-registered settings route once a plugin registers it", () => {
    function PluginSettingsPage() {
      return null;
    }
    pluginRegistry
      .forPlugin(PLUGIN_ID)
      .registerSettingsRoute(PLUGIN_SETTINGS_PATH, PluginSettingsPage);

    const route = renderSettingsRoute(PLUGIN_SETTINGS_PATH);

    expect(isValidElement(route)).toBe(true);
    expect((route as ReactElement).type).toBe(PluginSettingsPage);
  });

  it("does not consult the registry for a path outside /settings/plugins/", () => {
    function ShouldNotMatch() {
      return null;
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerSettingsRoute("/settings/general", ShouldNotMatch);

    const route = renderSettingsRoute("/settings/general");

    // "/settings/general" is a real static route (GeneralSettings), not the plugin's.
    expect((route as ReactElement).type).not.toBe(ShouldNotMatch);
  });
});

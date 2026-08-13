import { afterEach, describe, expect, it } from "vitest";
import { pluginRegistry } from "./registry";

const PRIMARY_PLUGIN_ID = "plugin-a";
const SECONDARY_PLUGIN_ID = "plugin-b";
const SOURCE_CONTROL_PROVIDER_ID = "source-control";

const registration = {
  id: SOURCE_CONTROL_PROVIDER_ID,
  label: "Source Control",
  description: "Configure source control.",
  Component: () => null,
};

describe("pluginRegistry — integration settings", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
    pluginRegistry.unregisterPlugin(SECONDARY_PLUGIN_ID);
  });

  it("exposes native integration settings registration to scoped plugins", () => {
    const scoped = pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID);

    expect("registerIntegrationSettings" in scoped).toBe(true);
  });

  it("keeps a contribution with its active plugin owner", () => {
    function Settings() {
      return null;
    }

    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerIntegrationSettings({
      ...registration,
      icon: "cloud",
      Component: Settings,
    });

    expect(pluginRegistry.getIntegrationSetting(SOURCE_CONTROL_PROVIDER_ID)).toEqual({
      pluginId: PRIMARY_PLUGIN_ID,
      ...registration,
      icon: "cloud",
      Component: Settings,
    });
  });

  it("rejects duplicate active ownership without replacing the owner", () => {
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerIntegrationSettings(registration);

    expect(() =>
      pluginRegistry.forPlugin(SECONDARY_PLUGIN_ID).registerIntegrationSettings(registration),
    ).toThrow(
      `integration settings "${SOURCE_CONTROL_PROVIDER_ID}" is already owned by "${PRIMARY_PLUGIN_ID}"`,
    );
    expect(pluginRegistry.getIntegrationSetting(SOURCE_CONTROL_PROVIDER_ID)?.pluginId).toBe(
      PRIMARY_PLUGIN_ID,
    );
  });

  it("rejects core integration IDs and path-unsafe IDs", () => {
    const scoped = pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID);

    expect(() => scoped.registerIntegrationSettings({ ...registration, id: "github" })).toThrow(
      'integration settings id "github" is reserved by the host',
    );
    expect(() =>
      scoped.registerIntegrationSettings({ ...registration, id: "unsafe/path" }),
    ).toThrow('integration settings id "unsafe/path" must be a URL-safe slug');
  });

  it("revokes on unload and lets a successor claim the id", () => {
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerIntegrationSettings(registration);

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
    pluginRegistry.forPlugin(SECONDARY_PLUGIN_ID).registerIntegrationSettings(registration);

    expect(pluginRegistry.getIntegrationSetting(SOURCE_CONTROL_PROVIDER_ID)?.pluginId).toBe(
      SECONDARY_PLUGIN_ID,
    );
  });
});

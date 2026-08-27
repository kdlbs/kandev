import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "./registry";

const PRIMARY_PLUGIN_ID = "plugin-a";
const SECONDARY_PLUGIN_ID = "plugin-b";
const SOURCE_CONTROL_PROVIDER_ID = "source-control";
const SECOND_SOURCE_CONTROL_PROVIDER_ID = "source-control-secondary";
const WORKSPACE_ID = "workspace-1";
const OTHER_WORKSPACE_ID = "workspace-2";

const registration = {
  id: SOURCE_CONTROL_PROVIDER_ID,
  label: "Source Control",
  description: "Configure source control.",
  Component: () => null,
};

function setIntegrationEnabled(
  pluginId: string,
  integrationId: string,
  workspaceId: string,
  enabled: boolean,
): void {
  (
    pluginRegistry.setIntegrationEnabled.bind(pluginRegistry) as unknown as (
      pluginId: string,
      integrationId: string,
      workspaceId: string,
      enabled: boolean,
    ) => void
  )(pluginId, integrationId, workspaceId, enabled);
}

function isIntegrationEnabled(integrationId: string, workspaceId: string): boolean {
  return (
    pluginRegistry.isIntegrationEnabled.bind(pluginRegistry) as unknown as (
      integrationId: string,
      workspaceId: string,
    ) => boolean
  )(integrationId, workspaceId);
}

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

  it("keeps enabled state independent for each registration and workspace", () => {
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerIntegrationSettings(registration);
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerIntegrationSettings({
      ...registration,
      id: SECOND_SOURCE_CONTROL_PROVIDER_ID,
    });

    setIntegrationEnabled(PRIMARY_PLUGIN_ID, SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID, true);
    setIntegrationEnabled(
      PRIMARY_PLUGIN_ID,
      SECOND_SOURCE_CONTROL_PROVIDER_ID,
      OTHER_WORKSPACE_ID,
      true,
    );

    expect(isIntegrationEnabled(SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID)).toBe(true);
    expect(isIntegrationEnabled(SOURCE_CONTROL_PROVIDER_ID, OTHER_WORKSPACE_ID)).toBe(false);
    expect(isIntegrationEnabled(SECOND_SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID)).toBe(false);
    expect(isIntegrationEnabled(SECOND_SOURCE_CONTROL_PROVIDER_ID, OTHER_WORKSPACE_ID)).toBe(true);
  });

  it("rejects state updates for an integration owned by another plugin", () => {
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerIntegrationSettings(registration);
    pluginRegistry.forPlugin(SECONDARY_PLUGIN_ID).registerIntegrationSettings({
      ...registration,
      id: SECOND_SOURCE_CONTROL_PROVIDER_ID,
    });

    const listener = vi.fn();
    const unsubscribe = pluginRegistry.subscribe(listener);

    setIntegrationEnabled(PRIMARY_PLUGIN_ID, SECOND_SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID, true);

    expect(listener).not.toHaveBeenCalled();
    expect(isIntegrationEnabled(SECOND_SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID)).toBe(false);
    unsubscribe();
  });

  it("suppresses unchanged notifications and clears state on unload", () => {
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerIntegrationSettings(registration);
    const listener = vi.fn();
    const unsubscribe = pluginRegistry.subscribe(listener);

    setIntegrationEnabled(PRIMARY_PLUGIN_ID, SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID, true);
    setIntegrationEnabled(PRIMARY_PLUGIN_ID, SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID, true);

    expect(listener).toHaveBeenCalledTimes(1);
    expect(isIntegrationEnabled(SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID)).toBe(true);

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);

    expect(isIntegrationEnabled(SOURCE_CONTROL_PROVIDER_ID, WORKSPACE_ID)).toBe(false);
    expect(listener).toHaveBeenCalledTimes(2);
    unsubscribe();
  });
});

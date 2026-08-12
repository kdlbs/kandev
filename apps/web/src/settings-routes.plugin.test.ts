import { createElement, isValidElement, type ReactElement, type ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { renderSettingsRoute } from "./settings-routes";

vi.mock("@/components/settings/workspaces/workspace-settings-shell", () => ({
  WorkspaceSettingsShell: ({ children }: { children: ReactNode }) => children,
}));

const PLUGIN_ID = "plugin-a";
const PLUGIN_SETTINGS_PATH = "/settings/plugins/plugin-a/config";
const PLUGIN_INTEGRATION_ID = "source-control";

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
      return "PluginSettingsPage:rendered";
    }
    pluginRegistry
      .forPlugin(PLUGIN_ID)
      .registerSettingsRoute(PLUGIN_SETTINGS_PATH, PluginSettingsPage);

    const route = renderSettingsRoute(PLUGIN_SETTINGS_PATH);

    expect(isValidElement(route)).toBe(true);
    render(route as ReactElement);
    expect(screen.getByText("PluginSettingsPage:rendered")).not.toBeNull();
    cleanup();
  });

  it("renders a fallback instead of throwing when the plugin settings route component throws", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    function ThrowingSettingsPage(): never {
      throw new Error("boom");
    }
    pluginRegistry
      .forPlugin(PLUGIN_ID)
      .registerSettingsRoute(PLUGIN_SETTINGS_PATH, ThrowingSettingsPage);

    const route = renderSettingsRoute(PLUGIN_SETTINGS_PATH);

    render(route as ReactElement);
    expect(screen.getByText(/this plugin page failed to load/i)).not.toBeNull();
    cleanup();
    errorSpy.mockRestore();
  });

  it("does not consult the registry for a path outside /settings/plugins/", () => {
    function ShouldNotMatch() {
      return null;
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerSettingsRoute("/settings/general", ShouldNotMatch);

    const route = renderSettingsRoute("/settings/general");

    // "/settings/general" is a first-party route (a redirect to its first page),
    // not the plugin's.
    expect((route as ReactElement).type).not.toBe(ShouldNotMatch);
  });
});

describe("renderSettingsRoute — plugin integration settings", () => {
  afterEach(() => {
    cleanupPlugins(PLUGIN_ID);
    cleanup();
  });

  it("redirects a registered integration global route into the active workspace", () => {
    function IntegrationSettings({ workspaceId }: { workspaceId?: string }) {
      return `PluginIntegration:${workspaceId ?? "global"}`;
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
      id: PLUGIN_INTEGRATION_ID,
      label: "Source Control",
      description: "Configure source control.",
      Component: IntegrationSettings,
    });

    const route = renderSettingsRoute(
      `/settings/integrations/${PLUGIN_INTEGRATION_ID}`,
    ) as ReactElement;

    expect((route.type as { name?: string }).name).toBe("ActiveWorkspaceSectionRedirect");
    expect(route.props).toEqual({ section: `integrations/${PLUGIN_INTEGRATION_ID}` });
  });

  it("passes the decoded workspace id to a registered integration", () => {
    function IntegrationSettings({ workspaceId }: { workspaceId?: string }) {
      return `PluginIntegration:${workspaceId}`;
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
      id: PLUGIN_INTEGRATION_ID,
      label: "Source Control",
      description: "Configure source control.",
      Component: IntegrationSettings,
    });

    render(
      renderSettingsRoute(
        `/settings/workspaces/workspace%20one/integrations/${PLUGIN_INTEGRATION_ID}`,
      ) as ReactElement,
    );

    expect(screen.getByText("PluginIntegration:workspace one")).not.toBeNull();
  });

  it("wraps plugin content in the native integration settings section", () => {
    function PluginProviderIcon({ className }: { className?: string }) {
      return createElement("svg", { className, "data-testid": "plugin-provider-icon" });
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
      id: PLUGIN_INTEGRATION_ID,
      label: "Acme Source Control",
      description: "Configure Acme source control.",
      icon: PluginProviderIcon,
      Component: () => "Plugin-owned settings cards",
    });

    render(
      renderSettingsRoute(
        `/settings/workspaces/workspace-1/integrations/${PLUGIN_INTEGRATION_ID}`,
      ) as ReactElement,
    );

    expect(screen.getByRole("heading", { name: "Acme Source Control" })).not.toBeNull();
    expect(screen.getByText("Configure Acme source control.")).not.toBeNull();
    expect(screen.getByTestId("plugin-provider-icon")).not.toBeNull();
    expect(screen.getByText("Plugin-owned settings cards")).not.toBeNull();
  });

  it("contains errors thrown by a plugin integration settings component", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    function ThrowingIntegration(): never {
      throw new Error("boom");
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
      id: PLUGIN_INTEGRATION_ID,
      label: "Source Control",
      description: "Configure source control.",
      Component: ThrowingIntegration,
    });

    render(
      renderSettingsRoute(
        `/settings/workspaces/workspace-1/integrations/${PLUGIN_INTEGRATION_ID}`,
      ) as ReactElement,
    );

    expect(screen.getByText(/this plugin page failed to load/i)).not.toBeNull();
    errorSpy.mockRestore();
  });
});

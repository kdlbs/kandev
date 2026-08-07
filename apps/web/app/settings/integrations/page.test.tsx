import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import IntegrationsIndexPage from "./page";

const PLUGIN_ID = "plugin-source-control";

function registerIntegration() {
  pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
    id: "source-control",
    label: "Source Control",
    description: "Connect a source-control provider.",
    icon: "cloud",
    Component: () => null,
  });
}

describe("IntegrationsIndexPage plugin contributions", () => {
  afterEach(() => {
    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));
    cleanup();
  });

  it("renders a plugin contribution beside native integrations", () => {
    registerIntegration();

    render(<IntegrationsIndexPage />);

    const link = screen.getByRole("link", { name: /source control/i });
    expect(link.getAttribute("href")).toBe("/settings/integrations/source-control");
    expect(screen.getByText("Connect a source-control provider.")).not.toBeNull();
  });

  it("uses the workspace-scoped native integration path", () => {
    registerIntegration();

    render(<IntegrationsIndexPage workspaceId="workspace one" />);

    expect(screen.getByRole("link", { name: /source control/i }).getAttribute("href")).toBe(
      "/settings/workspace/workspace%20one/integrations/source-control",
    );
  });

  it("removes the contribution reactively when its plugin unloads", () => {
    registerIntegration();
    render(<IntegrationsIndexPage />);
    expect(screen.getByRole("link", { name: /source control/i })).not.toBeNull();

    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

    expect(screen.queryByRole("link", { name: /source control/i })).toBeNull();
  });
});

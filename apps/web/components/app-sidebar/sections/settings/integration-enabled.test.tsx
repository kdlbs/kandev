import { cleanup, render, screen } from "@testing-library/react";
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { pluginRegistry } from "@/lib/plugins/registry";
import { IntegrationEnabledBadgeFor, IntegrationsEnabledProvider } from "./integration-enabled";

vi.mock("@/hooks/domains/integrations/use-enabled-integrations", () => ({
  useEnabledIntegrations: () => new Set<string>(),
}));

const PLUGIN_ID = "plugin-integration-badges";
const FIRST_ID = "first-provider";
const SECOND_ID = "second-provider";
const FIRST_WORKSPACE_ID = "workspace-1";
const SECOND_WORKSPACE_ID = "workspace-2";

function publish(integrationId: string, workspaceId: string, enabled: boolean) {
  (
    pluginRegistry.setIntegrationEnabled.bind(pluginRegistry) as unknown as (
      pluginId: string,
      integrationId: string,
      workspaceId: string,
      enabled: boolean,
    ) => void
  )(PLUGIN_ID, integrationId, workspaceId, enabled);
}

function Badge({ integrationId, workspaceId }: { integrationId: string; workspaceId: string }) {
  return (
    <IntegrationsEnabledProvider workspaceId={workspaceId}>
      <IntegrationEnabledBadgeFor slug={integrationId} />
    </IntegrationsEnabledProvider>
  );
}

describe("plugin integration enabled badges", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  function registerIntegrations() {
    pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
      id: FIRST_ID,
      label: "First provider",
      description: "First provider settings.",
      Component: () => null,
    });
    pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
      id: SECOND_ID,
      label: "Second provider",
      description: "Second provider settings.",
      Component: () => null,
    });
  }

  it("resolves each registration independently and keeps workspace state isolated", () => {
    registerIntegrations();
    publish(FIRST_ID, FIRST_WORKSPACE_ID, true);
    publish(SECOND_ID, FIRST_WORKSPACE_ID, false);
    publish(FIRST_ID, SECOND_WORKSPACE_ID, false);

    render(
      <>
        <div data-testid="first-ws-1">
          <Badge integrationId={FIRST_ID} workspaceId={FIRST_WORKSPACE_ID} />
        </div>
        <div data-testid="second-ws-1">
          <Badge integrationId={SECOND_ID} workspaceId={FIRST_WORKSPACE_ID} />
        </div>
        <div data-testid="first-ws-2">
          <Badge integrationId={FIRST_ID} workspaceId={SECOND_WORKSPACE_ID} />
        </div>
      </>,
    );

    expect(screen.getByTestId("first-ws-1").textContent).toContain("Enabled");
    expect(screen.getByTestId("second-ws-1").textContent).not.toContain("Enabled");
    expect(screen.getByTestId("first-ws-2").textContent).not.toContain("Enabled");
  });

  it("updates a badge when the registry publishes a new value", () => {
    registerIntegrations();
    publish(FIRST_ID, FIRST_WORKSPACE_ID, false);
    render(<Badge integrationId={FIRST_ID} workspaceId={FIRST_WORKSPACE_ID} />);

    expect(screen.queryByText("Enabled")).toBeNull();

    act(() => publish(FIRST_ID, FIRST_WORKSPACE_ID, true));

    expect(screen.getByText("Enabled")).not.toBeNull();
  });

  it("does not render a plugin badge without a workspace context", () => {
    registerIntegrations();
    publish(FIRST_ID, FIRST_WORKSPACE_ID, true);

    render(<IntegrationEnabledBadgeFor slug={FIRST_ID} />);

    expect(document.body.textContent).not.toContain("Enabled");
  });
});

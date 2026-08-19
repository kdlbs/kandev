import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { pluginRegistry } from "@/lib/plugins/registry";
import { renderPluginIntegrationSettings } from "./plugin-integration-settings-route";

const PLUGIN_ID = "plugin-route-test";
const INTEGRATION_ID = "route-provider";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("plugin integration settings route", () => {
  it("renders the optional action with the routed workspace id", () => {
    const actionWorkspaceIds: string[] = [];
    const Action = vi.fn(({ workspaceId }: { workspaceId?: string }) => {
      actionWorkspaceIds.push(workspaceId ?? "");
      return null;
    });

    pluginRegistry.forPlugin(PLUGIN_ID).registerIntegrationSettings({
      id: INTEGRATION_ID,
      label: "Route provider",
      description: "Route provider settings.",
      Component: () => null,
      action: Action,
    });

    const page = renderPluginIntegrationSettings(INTEGRATION_ID, "workspace-from-route");
    render(page);

    expect(Action.mock.calls[0]?.[0]).toEqual({
      workspaceId: "workspace-from-route",
      surface: "detail",
    });
    expect(actionWorkspaceIds).toEqual(["workspace-from-route"]);
  });
});

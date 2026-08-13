import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { pluginRegistry } from "@/lib/plugins/registry";

const state = {
  workspaces: { items: [{ id: "ws-1", name: "Default" }] },
  availableAgents: { items: [] },
  settingsAgents: { items: [] },
  executors: { items: [] },
  automations: { items: [] },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

import { useSettingsBreadcrumbs } from "./use-settings-breadcrumbs";

describe("useSettingsBreadcrumbs plugin integrations", () => {
  afterEach(() => pluginRegistry.unregisterPlugin("plugin-source-control"));

  it("uses and reactively revokes the registered integration label", () => {
    pluginRegistry.forPlugin("plugin-source-control").registerIntegrationSettings({
      id: "source-control",
      label: "Acme Source Control",
      description: "Configure Acme source control.",
      Component: () => null,
    });
    const { result } = renderHook(() =>
      useSettingsBreadcrumbs("/settings/workspaces/ws-1/integrations/source-control"),
    );

    expect(result.current.title).toBe("Acme Source Control");

    act(() => pluginRegistry.unregisterPlugin("plugin-source-control"));

    expect(result.current.title).toBe("Integrations");
  });
});

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { usePluginActionSurface } from "@/components/plugins/plugin-action-surface";
import { pluginRegistry } from "@/lib/plugins/registry";
import { AppSidebarWorkspaceActions } from "./app-sidebar-workspace-actions";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("plugin-a");
});

describe("AppSidebarWorkspaceActions", () => {
  it.each(["desktop", "mobile"] as const)(
    "provides the sidebar action surface for %s",
    (presentation) => {
      pluginRegistry.forPlugin("plugin-a").registerComponent("sidebar-workspace-actions", () => {
        const surface = usePluginActionSurface();
        return (
          <div
            data-testid="plugin-action-surface"
            data-surface={surface?.surface}
            data-presentation={surface?.presentation}
          />
        );
      });

      render(<AppSidebarWorkspaceActions workspaceId="workspace-1" presentation={presentation} />);

      expect(screen.getByTestId("plugin-action-surface").getAttribute("data-surface")).toBe(
        "sidebar",
      );
      expect(screen.getByTestId("plugin-action-surface").getAttribute("data-presentation")).toBe(
        presentation,
      );
    },
  );
});

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { usePluginActionSurface } from "@/components/plugins/plugin-action-surface";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { AppStatusBarSlotProps } from "@/lib/plugins/types";
import { AppStatusBarPluginContribution } from "./app-status-bar-plugin-slots";

const SLOT = "app-status-bar-right";

const slotProps = {
  placement: "right",
  presentation: "mobile-drawer",
  density: "compact",
  pathname: "/tasks/task-1",
  activeWorkspaceId: "workspace-1",
  activeTaskId: "task-1",
  activeSessionId: "session-1",
} satisfies AppStatusBarSlotProps;

describe("AppStatusBarPluginContribution", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin("plugin-a");
  });

  it("forwards the exact status-bar context to registered plugins", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent(SLOT, ({ slotProps: received }) => (
        <output data-testid="status-slot-props">{JSON.stringify(received)}</output>
      ));

    const registration = pluginRegistry.getSlotRegistrations(SLOT)[0];
    render(<AppStatusBarPluginContribution {...slotProps} registration={registration} />);

    expect(JSON.parse(screen.getByTestId("status-slot-props").textContent ?? "null")).toEqual(
      slotProps,
    );
  });

  it("renders the slot matching its left-side placement", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent("app-status-bar-left", () => <div data-testid="left-status-slot" />);

    const registration = pluginRegistry.getSlotRegistrations("app-status-bar-left")[0];
    render(
      <AppStatusBarPluginContribution
        {...slotProps}
        placement="left"
        registration={registration}
      />,
    );

    expect(screen.getByTestId("left-status-slot")).not.toBeNull();
  });

  it.each([
    ["bar", "full", "status-bar", "desktop"],
    ["mobile-drawer", "compact", "status-drawer", "mobile"],
  ] as const)(
    "provides the %s action surface",
    (presentation, density, surfaceName, actionPresentation) => {
      pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, () => {
        const surface = usePluginActionSurface();
        return (
          <div
            data-testid="status-action-surface"
            data-surface={surface?.surface}
            data-presentation={surface?.presentation}
          />
        );
      });

      const registration = pluginRegistry.getSlotRegistrations(SLOT)[0];
      render(
        <AppStatusBarPluginContribution
          {...slotProps}
          presentation={presentation}
          density={density}
          registration={registration}
        />,
      );

      expect(screen.getByTestId("status-action-surface").getAttribute("data-surface")).toBe(
        surfaceName,
      );
      expect(screen.getByTestId("status-action-surface").getAttribute("data-presentation")).toBe(
        actionPresentation,
      );
    },
  );
});

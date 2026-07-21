import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import { AppStatusBar } from "./app-status-bar";

const LEFT_PLUGIN_ID = "left-plugin";
const RIGHT_PLUGIN_ID = "right-plugin";

describe("AppStatusBar", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin(LEFT_PLUGIN_ID);
    pluginRegistry.unregisterPlugin(RIGHT_PLUGIN_ID);
  });

  it("keeps a 24px in-flow footer and renders both live plugin regions", () => {
    pluginRegistry
      .forPlugin(LEFT_PLUGIN_ID)
      .registerComponent("app-status-bar-left", () => (
        <span data-testid={LEFT_PLUGIN_ID}>Left</span>
      ));
    pluginRegistry
      .forPlugin(RIGHT_PLUGIN_ID)
      .registerComponent("app-status-bar-right", () => (
        <span data-testid={RIGHT_PLUGIN_ID}>Right</span>
      ));

    render(
      <StateProvider>
        <TooltipProvider>
          <AppStatusBar
            pathname="/tasks/task-1"
            activeWorkspaceId="workspace-1"
            activeTaskId="task-1"
            activeSessionId="session-1"
            density="full"
          />
        </TooltipProvider>
      </StateProvider>,
    );

    expect(screen.getByTestId("app-status-bar").className).toContain("h-6");
    expect(screen.getByTestId(LEFT_PLUGIN_ID)).toBeTruthy();
    expect(screen.getByTestId(RIGHT_PLUGIN_ID)).toBeTruthy();
  });

  it("centers plugin regions inside the fixed-height bar", () => {
    pluginRegistry
      .forPlugin(LEFT_PLUGIN_ID)
      .registerComponent("app-status-bar-left", () => (
        <span data-testid={LEFT_PLUGIN_ID}>Left</span>
      ));

    render(
      <StateProvider>
        <TooltipProvider>
          <AppStatusBar
            pathname="/"
            activeWorkspaceId={null}
            activeTaskId={null}
            activeSessionId={null}
            density="full"
          />
        </TooltipProvider>
      </StateProvider>,
    );

    expect(screen.getByTestId("app-status-bar-left-plugins").className).toContain("items-center");
    expect(screen.getByTestId("app-status-bar-left-plugins").className).toContain("h-full");
  });
});

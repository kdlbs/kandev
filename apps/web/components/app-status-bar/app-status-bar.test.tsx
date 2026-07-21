import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import { AppStatusBar } from "./app-status-bar";

describe("AppStatusBar", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin("left-plugin");
    pluginRegistry.unregisterPlugin("right-plugin");
  });

  it("keeps a 24px in-flow footer and renders both live plugin regions", () => {
    pluginRegistry
      .forPlugin("left-plugin")
      .registerComponent("app-status-bar-left", () => <span data-testid="left-plugin">Left</span>);
    pluginRegistry
      .forPlugin("right-plugin")
      .registerComponent("app-status-bar-right", () => (
        <span data-testid="right-plugin">Right</span>
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
    expect(screen.getByTestId("left-plugin")).toBeTruthy();
    expect(screen.getByTestId("right-plugin")).toBeTruthy();
  });
});

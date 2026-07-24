import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { AppStatusBarSlotProps } from "@/lib/plugins/types";
import { AppStatusDrawer } from "./app-status-drawer";
import { APP_STATUS_CONNECTION_ID, APP_STATUS_METRICS_ID } from "./app-status-bar-order";

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DrawerContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  DrawerHeader: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  DrawerTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true }),
}));

const LEFT_PLUGIN_ID = "drawer-left";
const RIGHT_PLUGIN_ID = "drawer-right";

describe("AppStatusDrawer", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin(LEFT_PLUGIN_ID);
    pluginRegistry.unregisterPlugin(RIGHT_PLUGIN_ID);
  });

  it("mirrors saved left then right order as non-draggable 44px rows", () => {
    pluginRegistry
      .forPlugin(LEFT_PLUGIN_ID)
      .registerComponent("app-status-bar-left", ({ slotProps }) => {
        const props = slotProps as AppStatusBarSlotProps;
        return <span data-testid={LEFT_PLUGIN_ID}>{props.presentation}</span>;
      });
    pluginRegistry
      .forPlugin(RIGHT_PLUGIN_ID)
      .registerComponent("app-status-bar-right", ({ slotProps }) => {
        const props = slotProps as AppStatusBarSlotProps;
        return <span data-testid={RIGHT_PLUGIN_ID}>{props.presentation}</span>;
      });
    const leftId = `plugin:${LEFT_PLUGIN_ID}:app-status-bar-left:0`;
    const rightId = `plugin:${RIGHT_PLUGIN_ID}:app-status-bar-right:0`;

    render(
      <StateProvider
        initialState={{
          userSettings: {
            ...defaultSettingsState.userSettings,
            systemMetricsDisplay: { showInTopbar: true, simplified: false },
            appStatusBarOrder: {
              leftItemIds: [rightId, APP_STATUS_METRICS_ID],
              rightItemIds: [APP_STATUS_CONNECTION_ID, leftId],
            },
          },
        }}
      >
        <TooltipProvider>
          <AppStatusDrawer
            pathname="/tasks/task-1"
            activeWorkspaceId="workspace-1"
            activeTaskId="task-1"
            activeSessionId="session-1"
            open
            onOpenChange={() => {}}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    const rows = Array.from(
      screen
        .getByTestId("app-status-drawer")
        .querySelectorAll<HTMLElement>("[data-status-item-id]"),
    );
    expect(rows.map((row) => row.dataset.statusItemId)).toEqual([
      rightId,
      APP_STATUS_METRICS_ID,
      APP_STATUS_CONNECTION_ID,
      leftId,
    ]);
    expect(rows.every((row) => row.className.includes("min-h-11"))).toBe(true);
    expect(screen.getByTestId(LEFT_PLUGIN_ID).textContent).toBe("mobile-drawer");
    expect(screen.getByTestId(RIGHT_PLUGIN_ID).textContent).toBe("mobile-drawer");
  });

  it("collapses a plugin row when its contribution renders nothing", () => {
    pluginRegistry.forPlugin(RIGHT_PLUGIN_ID).registerComponent("app-status-bar-right", () => null);

    render(
      <StateProvider>
        <TooltipProvider>
          <AppStatusDrawer
            pathname="/"
            activeWorkspaceId={null}
            activeTaskId={null}
            activeSessionId={null}
            open
            onOpenChange={() => {}}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    const row = document.querySelector<HTMLElement>(
      `[data-status-item-id="plugin:${RIGHT_PLUGIN_ID}:app-status-bar-right:0"]`,
    );
    expect(row?.className).toContain("empty:hidden");
  });
});

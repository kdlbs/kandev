import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskChangeRequestLinkForm } from "@/components/integrations/task-change-request-link-form";
import { pluginModalManager } from "@/lib/plugins/modal-manager";
import { AppShell } from "./app-shell";

const mocks = vi.hoisted(() => ({
  passthrough: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock("@/components/app-sidebar/app-sidebar", () => ({ AppSidebar: () => null }));
vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  AppStatusSurfaceProvider: mocks.passthrough,
}));
vi.mock("@/components/command-panel", () => ({ CommandPanel: () => null }));
vi.mock("@/components/config-chat/config-chat-provider", () => ({
  ConfigChatProvider: mocks.passthrough,
}));
vi.mock("@/components/diff-worker-pool-provider", () => ({
  DiffWorkerPoolProvider: mocks.passthrough,
}));
vi.mock("@/components/desktop-command-host", () => ({ DesktopCommandHost: () => null }));
vi.mock("@/components/global-commands", () => ({ GlobalCommands: () => null }));
vi.mock("@/components/log-buffer-bridge", () => ({ LogBufferBridge: () => null }));
vi.mock("@/components/quick-chat/quick-chat-provider", () => ({
  QuickChatProvider: mocks.passthrough,
}));
vi.mock("@/components/task/recent-task-switcher", () => ({ RecentTaskSwitcher: () => null }));
vi.mock("@/components/session-failure-toast-bridge", () => ({
  SessionFailureToastBridge: () => null,
}));
vi.mock("@/components/task-deleted-toast-bridge", () => ({ TaskDeletedToastBridge: () => null }));
vi.mock("@/components/update-available-toast-bridge", () => ({
  UpdateAvailableToastBridge: () => null,
}));
vi.mock("@/components/sidebar-views-sync-bridge", () => ({ SidebarViewsSyncBridge: () => null }));
vi.mock("@/components/theme-provider", () => ({ ThemeProvider: mocks.passthrough }));
// Reads the store to derive the workspace's mode; this suite mounts the shell
// without a StateProvider, and only cares about provider nesting order.
vi.mock("@/components/workspace-scope-provider", () => ({
  WorkspaceScopeProvider: mocks.passthrough,
}));
vi.mock("@/components/ws-connector", () => ({ WebSocketConnector: () => null }));
vi.mock("@/lib/commands/command-registry", () => ({
  CommandRegistryProvider: mocks.passthrough,
}));
vi.mock("@kandev/ui/sonner", () => ({ Toaster: () => null }));
vi.mock("@kandev/ui/tooltip", () => ({ TooltipProvider: mocks.passthrough }));

describe("AppShell plugin modal topology", () => {
  afterEach(() => {
    cleanup();
    pluginModalManager.closeAllForPlugin("bitbucket");
    Reflect.deleteProperty(navigator, "windowControlsOverlay");
  });

  it("renders host task-link forms inside the shared toast provider", () => {
    pluginModalManager.openTaskLinkDialog("bitbucket", {
      title: "Link Bitbucket pull request",
      content: () => (
        <TaskChangeRequestLinkForm
          inputLabel="Pull request"
          emptyError="Enter a Bitbucket pull request URL or key."
          failureMessage="Failed to link Bitbucket pull request."
          successMessage="Bitbucket pull request linked"
          onSubmit={vi.fn().mockResolvedValue(undefined)}
          onCancel={() => undefined}
          onSuccess={() => undefined}
        />
      ),
    });

    render(<AppShell>App content</AppShell>);

    expect(screen.getByLabelText("Pull request")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Save" })).not.toBeNull();
  });

  it("publishes visible window-controls-overlay geometry on the root shell", () => {
    Object.defineProperty(navigator, "windowControlsOverlay", {
      configurable: true,
      value: {
        visible: true,
        getTitlebarAreaRect: () => ({ x: 72, y: 0, width: 1448, height: 40 }),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      },
    });

    render(<AppShell>App content</AppShell>);

    const shell = screen.getByTestId("app-shell");
    expect(shell.getAttribute("data-window-controls-overlay")).toBe("visible");
    expect(shell.style.getPropertyValue("--titlebar-area-x")).toBe("72px");
    expect(shell.style.getPropertyValue("--titlebar-area-width")).toBe("1448px");
    expect(shell.style.getPropertyValue("--titlebar-area-height")).toBe("40px");
  });
});

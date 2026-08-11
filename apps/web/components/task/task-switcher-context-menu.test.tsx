import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";

const PLUGIN_ID = "kandev-plugin-tags";
const WORKSPACE_ID = "ws-1";
const TASK: TaskSwitcherItem = {
  id: "task-1",
  title: "Fix the bug",
  workflowStepId: "step-1",
};

function Providers({
  children,
  workspaceId = WORKSPACE_ID,
}: {
  children: React.ReactNode;
  workspaceId?: string | null;
}) {
  return (
    <StateProvider initialState={{ workspaces: { items: [], activeId: workspaceId } }}>
      <ToastProvider>{children}</ToastProvider>
    </StateProvider>
  );
}

function renderMenu(
  overrides: Partial<ComponentProps<typeof TaskItemWithContextMenu>> = {},
  workspaceId: string | null = WORKSPACE_ID,
) {
  return render(
    <Providers workspaceId={workspaceId}>
      <TaskItemWithContextMenu task={TASK} {...overrides}>
        <div>{(overrides.task ?? TASK).title}</div>
      </TaskItemWithContextMenu>
    </Providers>,
  );
}

function registerPrimaryAction(
  overrides: {
    run?: (context: PluginTaskMenuContext) => void | Promise<void>;
    visible?: (context: PluginTaskMenuContext) => boolean;
  } = {},
) {
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: "quick-tag",
    label: "Quick tag",
    icon: <span data-testid="quick-tag-icon" />,
    group: "primary",
    run: overrides.run ?? vi.fn(),
    ...(overrides.visible ? { visible: overrides.visible } : {}),
  });
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

describe("TaskItemWithContextMenu — plugin 'primary' menu actions", () => {
  it("renders a registered action with its label and icon", () => {
    registerPrimaryAction();

    renderMenu({ onLinkPullRequest: vi.fn() });
    fireEvent.contextMenu(screen.getByText(TASK.title));

    const item = screen.getByRole("menuitem", { name: "Quick tag" });
    expect(item.querySelector('[data-testid="quick-tag-icon"]')).not.toBeNull();
  });

  it("positions the action immediately before the Link submenu", () => {
    registerPrimaryAction();

    renderMenu({ onLinkPullRequest: vi.fn() });
    fireEvent.contextMenu(screen.getByText(TASK.title));

    const labels = screen.getAllByRole("menuitem").map((el) => el.textContent);
    const quickTagIndex = labels.findIndex((text) => text?.includes("Quick tag"));
    const linkIndex = labels.findIndex((text) => text?.includes("Link"));

    expect(quickTagIndex).toBeGreaterThanOrEqual(0);
    expect(linkIndex).toBeGreaterThanOrEqual(0);
    expect(linkIndex).toBe(quickTagIndex + 1);
  });

  it("is absent from the bulk-selection menu when multiple rows are selected", () => {
    registerPrimaryAction();

    renderMenu({
      onLinkPullRequest: vi.fn(),
      selectedTaskIds: new Set([TASK.id, "task-2"]),
      onBulkArchive: vi.fn(),
    });
    fireEvent.contextMenu(screen.getByText(TASK.title));

    expect(screen.queryByRole("menuitem", { name: "Quick tag" })).toBeNull();
  });

  it("with no plugin registered, renders the menu with no stray separator", () => {
    renderMenu({ onLinkPullRequest: vi.fn() });
    fireEvent.contextMenu(screen.getByText(TASK.title));

    expect(screen.queryByText("Quick tag")).toBeNull();
    expect(screen.getByRole("menuitem", { name: "Link" })).not.toBeNull();
    expect(screen.queryAllByRole("separator")).toHaveLength(0);
  });

  it("skips rendering plugin entries entirely when there is no active workspace", () => {
    registerPrimaryAction();

    renderMenu({ onLinkPullRequest: vi.fn() }, null);
    fireEvent.contextMenu(screen.getByText(TASK.title));

    expect(screen.queryByText("Quick tag")).toBeNull();
    expect(screen.getByRole("menuitem", { name: "Link" })).not.toBeNull();
  });

  it("invokes run() with this row's workspaceId/taskId/taskTitle/workflowStepId/presentation", async () => {
    const run = vi.fn().mockResolvedValue(undefined);
    registerPrimaryAction({ run });

    renderMenu({ onLinkPullRequest: vi.fn() });
    fireEvent.contextMenu(screen.getByText(TASK.title));
    fireEvent.click(screen.getByRole("menuitem", { name: "Quick tag" }));

    await Promise.resolve();
    expect(run).toHaveBeenCalledWith({
      workspaceId: WORKSPACE_ID,
      taskId: TASK.id,
      taskTitle: TASK.title,
      workflowStepId: TASK.workflowStepId,
      presentation: "desktop",
    });
  });
});

describe("TaskItemWithContextMenu — plugin 'primary' action error handling", () => {
  it("treats a throwing visible() as hidden and logs it", () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    registerPrimaryAction({
      visible: () => {
        throw new Error("visible() blew up");
      },
    });

    renderMenu({ onLinkPullRequest: vi.fn() });
    fireEvent.contextMenu(screen.getByText(TASK.title));

    expect(screen.queryByText("Quick tag")).toBeNull();
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining(`${PLUGIN_ID}:quick-tag`),
      expect.any(Error),
    );
    consoleErrorSpy.mockRestore();
  });

  it("catches a synchronously-throwing run(), logs it, and still closes the menu", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const run = vi.fn(() => {
      throw new Error("run() blew up synchronously");
    });
    registerPrimaryAction({ run });

    renderMenu({ onLinkPullRequest: vi.fn() });
    fireEvent.contextMenu(screen.getByText(TASK.title));
    expect(() =>
      fireEvent.click(screen.getByRole("menuitem", { name: "Quick tag" })),
    ).not.toThrow();

    await Promise.resolve();
    await Promise.resolve();
    expect(run).toHaveBeenCalledTimes(1);
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining(`${PLUGIN_ID}:quick-tag`),
      expect.any(Error),
    );
    // Radix closes the menu on select regardless of the handler outcome.
    expect(screen.queryByText("Quick tag")).toBeNull();
    consoleErrorSpy.mockRestore();
  });

  it("catches a rejecting run() without throwing and still closes the menu", async () => {
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const run = vi.fn().mockRejectedValue(new Error("boom"));
    registerPrimaryAction({ run });

    renderMenu({ onLinkPullRequest: vi.fn() });
    fireEvent.contextMenu(screen.getByText(TASK.title));
    expect(() =>
      fireEvent.click(screen.getByRole("menuitem", { name: "Quick tag" })),
    ).not.toThrow();

    await Promise.resolve();
    await Promise.resolve();
    consoleErrorSpy.mockRestore();
  });
});

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { pluginRegistry } from "@/lib/plugins/registry";
import { AppStatusBar } from "./app-status-bar";
import { APP_STATUS_CONNECTION_ID, APP_STATUS_METRICS_ID } from "./app-status-bar-order";

vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

const LEFT_PLUGIN_ID = "left-plugin";
const RIGHT_PLUGIN_ID = "right-plugin";
const APP_STATUS_BAR_TEST_ID = "app-status-bar";
const APP_STATUS_LEFT_SLOT = "app-status-bar-left";

beforeEach(() => {
  vi.mocked(updateUserSettings)
    .mockReset()
    .mockResolvedValue({ settings: {} } as never);
  vi.mocked(toast.error).mockReset();
});

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(LEFT_PLUGIN_ID);
  pluginRegistry.unregisterPlugin(RIGHT_PLUGIN_ID);
});

describe("AppStatusBar rendering", () => {
  it("keeps a 24px in-flow footer and renders both live plugin regions", () => {
    pluginRegistry
      .forPlugin(LEFT_PLUGIN_ID)
      .registerComponent(APP_STATUS_LEFT_SLOT, () => (
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

    expect(screen.getByTestId(APP_STATUS_BAR_TEST_ID).className).toContain("h-6");
    expect(screen.getByTestId(LEFT_PLUGIN_ID)).toBeTruthy();
    expect(screen.getByTestId(RIGHT_PLUGIN_ID)).toBeTruthy();
  });

  it("centers plugin regions inside the fixed-height bar", () => {
    pluginRegistry
      .forPlugin(LEFT_PLUGIN_ID)
      .registerComponent(APP_STATUS_LEFT_SLOT, () => (
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
    expect(screen.getByTestId(APP_STATUS_BAR_TEST_ID).className).not.toContain("border-t");
    expect(screen.getByTestId(APP_STATUS_BAR_TEST_ID).className).toContain("before:h-px");
  });

  it("does not render an empty metrics item when the metrics preference is disabled", () => {
    renderBar();

    expect(document.querySelector(`[data-status-item-id="${APP_STATUS_METRICS_ID}"]`)).toBeNull();
  });
});

describe("AppStatusBar drag input", () => {
  it("moves an opaque plugin item across the spacer with Ctrl plus mouse drag", async () => {
    const onClick = vi.fn();
    pluginRegistry.forPlugin(LEFT_PLUGIN_ID).registerComponent(APP_STATUS_LEFT_SLOT, () => (
      <button type="button" onClick={onClick}>
        Plugin action
      </button>
    ));
    pluginRegistry
      .forPlugin(RIGHT_PLUGIN_ID)
      .registerComponent("app-status-bar-right", () => <span>Right</span>);

    renderBar();
    const leftId = `plugin:${LEFT_PLUGIN_ID}:app-status-bar-left:0`;
    const rightId = `plugin:${RIGHT_PLUGIN_ID}:app-status-bar-right:0`;
    const dragged = statusItem(leftId);
    setRect(statusItem(APP_STATUS_CONNECTION_ID), 0, 20);
    setRect(dragged, 20, 40);
    setRect(screen.getByTestId("app-status-bar-spacer"), 40, 80);
    setRect(statusItem(rightId), 80, 100);

    fireEvent.pointerDown(dragged, {
      pointerId: 1,
      pointerType: "mouse",
      ctrlKey: true,
      clientX: 30,
    });
    fireEvent.pointerMove(screen.getByTestId(APP_STATUS_BAR_TEST_ID), {
      pointerId: 1,
      pointerType: "mouse",
      ctrlKey: true,
      clientX: 85,
    });
    fireEvent.pointerUp(screen.getByTestId(APP_STATUS_BAR_TEST_ID), {
      pointerId: 1,
      pointerType: "mouse",
      ctrlKey: true,
      clientX: 85,
    });
    fireEvent.click(screen.getByRole("button", { name: "Plugin action" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({
        app_status_bar_order: {
          left_item_ids: [APP_STATUS_CONNECTION_ID],
          right_item_ids: [leftId, rightId],
        },
      }),
    );
    expect(statusItem(leftId).dataset.statusSide).toBe("right");
    expect(onClick).not.toHaveBeenCalled();
  });

  it("keeps normal plugin clicks and ignores plain mouse, touch, and sub-threshold movement", () => {
    const onClick = vi.fn();
    pluginRegistry.forPlugin(LEFT_PLUGIN_ID).registerComponent(APP_STATUS_LEFT_SLOT, () => (
      <button type="button" onClick={onClick}>
        Plugin action
      </button>
    ));
    renderBar();
    const item = statusItem(`plugin:${LEFT_PLUGIN_ID}:app-status-bar-left:0`);
    const bar = screen.getByTestId(APP_STATUS_BAR_TEST_ID);

    fireEvent.pointerDown(item, { pointerId: 1, pointerType: "mouse", clientX: 10 });
    fireEvent.pointerMove(bar, { pointerId: 1, pointerType: "mouse", clientX: 30 });
    fireEvent.pointerUp(bar, { pointerId: 1, pointerType: "mouse", clientX: 30 });
    fireEvent.pointerDown(item, {
      pointerId: 2,
      pointerType: "touch",
      ctrlKey: true,
      clientX: 10,
    });
    fireEvent.pointerMove(bar, { pointerId: 2, pointerType: "touch", clientX: 30 });
    fireEvent.pointerUp(bar, { pointerId: 2, pointerType: "touch", clientX: 30 });
    fireEvent.pointerDown(item, {
      pointerId: 3,
      pointerType: "mouse",
      metaKey: true,
      clientX: 10,
    });
    fireEvent.pointerMove(bar, { pointerId: 3, pointerType: "mouse", clientX: 13 });
    fireEvent.pointerUp(bar, { pointerId: 3, pointerType: "mouse", clientX: 13 });
    fireEvent.click(screen.getByRole("button", { name: "Plugin action" }));

    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(onClick).toHaveBeenCalledOnce();
  });
});

describe("AppStatusBar persistence and recovery", () => {
  it("restores the confirmed order and reports a rejected save", async () => {
    pluginRegistry
      .forPlugin(LEFT_PLUGIN_ID)
      .registerComponent(APP_STATUS_LEFT_SLOT, () => <span>Left</span>);
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError("rejected", 400, null));
    renderBar();
    const leftId = `plugin:${LEFT_PLUGIN_ID}:app-status-bar-left:0`;
    const dragged = statusItem(leftId);
    setRect(statusItem(APP_STATUS_CONNECTION_ID), 0, 20);
    setRect(dragged, 20, 40);
    setRect(screen.getByTestId("app-status-bar-spacer"), 40, 80);

    fireEvent.pointerDown(dragged, {
      pointerId: 4,
      pointerType: "mouse",
      ctrlKey: true,
      clientX: 30,
    });
    fireEvent.pointerMove(screen.getByTestId(APP_STATUS_BAR_TEST_ID), {
      pointerId: 4,
      pointerType: "mouse",
      ctrlKey: true,
      clientX: 85,
    });
    fireEvent.pointerUp(screen.getByTestId(APP_STATUS_BAR_TEST_ID), {
      pointerId: 4,
      pointerType: "mouse",
      ctrlKey: true,
      clientX: 85,
    });

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Could not save status bar order"),
    );
    expect(statusItem(leftId).dataset.statusSide).toBe("left");
  });

  it("resets a failed contribution boundary when the registration is replaced", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    pluginRegistry.forPlugin(LEFT_PLUGIN_ID).registerComponent(APP_STATUS_LEFT_SLOT, () => {
      throw new Error("plugin failed");
    });
    renderBar();

    act(() => {
      pluginRegistry.unregisterPlugin(LEFT_PLUGIN_ID);
      pluginRegistry
        .forPlugin(LEFT_PLUGIN_ID)
        .registerComponent(APP_STATUS_LEFT_SLOT, () => <span>Healthy replacement</span>);
    });

    expect(screen.getByText("Healthy replacement")).toBeTruthy();
    consoleError.mockRestore();
  });
});

function renderBar() {
  return render(
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
}

function statusItem(id: string): HTMLElement {
  const item = document.querySelector<HTMLElement>(`[data-status-item-id="${id}"]`);
  if (!item) throw new Error(`Missing status item ${id}`);
  return item;
}

function setRect(element: HTMLElement, left: number, right: number) {
  element.getBoundingClientRect = () =>
    ({
      left,
      right,
      width: right - left,
      top: 0,
      bottom: 24,
      height: 24,
      x: left,
      y: 0,
    }) as DOMRect;
}

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { RemoteCloudTooltip } from "./remote-cloud-tooltip";

const pointer = vi.hoisted(() => ({ touch: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => pointer.touch }));
afterEach(() => {
  cleanup();
  pointer.touch = false;
});

// @covers AC-EXECUTORS-TASK-STATUS-001.3
it("links the actual tooltip to the focused DOM trigger and dismisses on Escape", async () => {
  render(
    <TooltipProvider>
      <RemoteCloudTooltip
        taskId="task"
        executorType="k8s"
        status={{ remote_name: "test-pod", remote_state: "running" }}
      />
    </TooltipProvider>,
  );
  const trigger = screen.getByTestId("remote-executor-status-trigger");
  act(() => trigger.focus());
  const tooltip = await screen.findByRole("tooltip");
  expect(trigger.getAttribute("aria-describedby")).toBe(tooltip.id);
  fireEvent.keyDown(trigger, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("tooltip")).toBeNull());
});

// @covers AC-EXECUTORS-TASK-STATUS-001.4
it("opens and closes the real drawer without selecting its task", async () => {
  pointer.touch = true;
  const selectTask = vi.fn();
  render(
    <div onClick={selectTask}>
      <RemoteCloudTooltip
        taskId="task"
        executorType="k8s"
        status={{ remote_name: "touch-pod", remote_state: "running" }}
      />
    </div>,
  );
  const trigger = screen.getByTestId("remote-executor-status-trigger");
  act(() => trigger.focus());
  fireEvent.keyDown(trigger, { key: "Enter" });
  const drawer = await screen.findByRole("dialog");
  expect(trigger.getAttribute("aria-controls")).toBe(drawer.id);
  expect(selectTask).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() => expect(trigger.getAttribute("aria-expanded")).toBe("false"));
  expect(selectTask).not.toHaveBeenCalled();
});

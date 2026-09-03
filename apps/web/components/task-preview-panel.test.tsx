import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskPreviewPanel } from "./task-preview-panel";
import type { Task } from "./kanban-card";

afterEach(() => cleanup());

vi.mock("./task/preview-session-tabs", () => ({
  PreviewSessionTabs: () => <div data-testid="preview-session-tabs" />,
}));

const TRIGGER_TEST_ID = "task-preview-actions-menu";

const TASK: Task = {
  id: "task-1",
  title: "Fix the sidebar",
  workflowStepId: "step-1",
};

const CHILD_TASK: Task = {
  ...TASK,
  parentTaskId: "task-parent",
};

function renderPanel(ui: React.ReactNode) {
  return render(
    <ToastProvider>
      <StateProvider>{ui}</StateProvider>
    </ToastProvider>,
  );
}

function rerenderPanel(rerender: (ui: React.ReactNode) => void, ui: React.ReactNode) {
  rerender(
    <ToastProvider>
      <StateProvider>{ui}</StateProvider>
    </ToastProvider>,
  );
}

function getTrigger() {
  return screen.getByTestId(TRIGGER_TEST_ID);
}

function openMenu() {
  const trigger = getTrigger();
  fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
  fireEvent.click(trigger);
  return trigger;
}

function expectMenuOpen(open: boolean) {
  expect(getTrigger().getAttribute("aria-expanded")).toBe(String(open));
}

describe("TaskPreviewPanel actions menu trigger", () => {
  it("renders no trigger when the panel has no subject task", () => {
    renderPanel(<TaskPreviewPanel task={null} onClose={vi.fn()} />);

    expect(screen.queryByTestId(TRIGGER_TEST_ID)).toBeNull();
  });

  it("renders the trigger before Maximize, with the More options accessible name", () => {
    renderPanel(<TaskPreviewPanel task={TASK} onClose={vi.fn()} onMaximize={vi.fn()} />);

    const trigger = getTrigger();
    expect(trigger.getAttribute("aria-label")).toBe("More options");
    expect(trigger.getAttribute("aria-haspopup")).toBe("menu");

    // AC-TASKS-TASK-ACTIONS-MENU-001.1: before Maximize, which is itself
    // before Close.
    const controls = screen.getAllByRole("button");
    const maximizeIndex = controls.findIndex((el) => el.title === "Open full page");
    const triggerIndex = controls.indexOf(trigger);
    expect(triggerIndex).toBeGreaterThanOrEqual(0);
    expect(triggerIndex).toBeLessThan(maximizeIndex);
  });

  it("opens a menu offering Detach from parent for a subject task with a parent", () => {
    renderPanel(<TaskPreviewPanel task={CHILD_TASK} onClose={vi.fn()} />);

    openMenu();

    expect(screen.getByRole("menuitem", { name: "Detach from parent" })).toBeTruthy();
  });

  it("offers no Detach from parent when the subject task has none", () => {
    renderPanel(<TaskPreviewPanel task={TASK} onClose={vi.fn()} />);

    openMenu();

    expect(screen.queryByRole("menuitem", { name: "Detach from parent" })).toBeNull();
  });
});

describe("TaskPreviewPanel actions menu — subject identity change (AC-TASKS-TASK-ACTIONS-MENU-004.5a)", () => {
  const OTHER_TASK: Task = { id: "task-2", title: "Other task", workflowStepId: "step-1" };

  it("closes an open menu, without re-targeting it, when the subject task's identifier changes", () => {
    const onActionsMenuOpenChange = vi.fn();
    const { rerender } = renderPanel(
      <TaskPreviewPanel
        task={TASK}
        onClose={vi.fn()}
        onActionsMenuOpenChange={onActionsMenuOpenChange}
      />,
    );
    openMenu();
    expectMenuOpen(true);
    onActionsMenuOpenChange.mockClear();

    rerenderPanel(
      rerender,
      <TaskPreviewPanel
        task={OTHER_TASK}
        onClose={vi.fn()}
        onActionsMenuOpenChange={onActionsMenuOpenChange}
      />,
    );

    expectMenuOpen(false);
    expect(onActionsMenuOpenChange).toHaveBeenCalledWith(false);
  });

  it("leaves an open menu open across a re-render that keeps the same subject identifier", () => {
    const { rerender } = renderPanel(<TaskPreviewPanel task={TASK} onClose={vi.fn()} />);
    openMenu();
    expectMenuOpen(true);

    // A field-only update to the same task (new object, same id) must not
    // close the menu.
    rerenderPanel(
      rerender,
      <TaskPreviewPanel task={{ ...TASK, title: "Fix the sidebar (renamed)" }} onClose={vi.fn()} />,
    );

    expectMenuOpen(true);
  });
});

import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskPreviewPanel } from "./task-preview-panel";
import type { Task } from "./kanban-card";

const detachTaskMock = vi.hoisted(() => vi.fn().mockResolvedValue({ id: "task-1" }));
vi.mock("@/lib/api/domains/kanban-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/kanban-api")>(
    "@/lib/api/domains/kanban-api",
  );
  return { ...actual, detachTask: detachTaskMock };
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  detachTaskMock.mockClear();
});

vi.mock("./task/preview-session-tabs", () => ({
  PreviewSessionTabs: () => <div data-testid="preview-session-tabs" />,
}));

const TRIGGER_TEST_ID = "task-preview-actions-menu";
const DETACH_MENU_ITEM_NAME = "Detach from parent";
const DETACH_CONFIRM_POPOVER_TEST_ID = "detach-task-confirm-popover";

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

    expect(screen.getByRole("menuitem", { name: DETACH_MENU_ITEM_NAME })).toBeTruthy();
  });

  it("offers no Detach from parent when the subject task has none", () => {
    renderPanel(<TaskPreviewPanel task={TASK} onClose={vi.fn()} />);

    openMenu();

    expect(screen.queryByRole("menuitem", { name: DETACH_MENU_ITEM_NAME })).toBeNull();
  });

  it("requests detach for the subject task and closes the confirmation once it completes (AC-TASKS-TASK-ACTIONS-MENU-003.10)", async () => {
    vi.useFakeTimers();
    renderPanel(<TaskPreviewPanel task={CHILD_TASK} onClose={vi.fn()} />);

    openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: DETACH_MENU_ITEM_NAME }));
    await act(async () => {
      vi.advanceTimersByTime(300);
    });
    expect(screen.getByTestId(DETACH_CONFIRM_POPOVER_TEST_ID)).toBeTruthy();

    fireEvent.click(screen.getByTestId("detach-task-confirm"));
    vi.useRealTimers();

    await waitFor(() => expect(detachTaskMock).toHaveBeenCalledWith(CHILD_TASK.id));
    await waitFor(() => expect(screen.queryByTestId(DETACH_CONFIRM_POPOVER_TEST_ID)).toBeNull());
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

describe("TaskPreviewPanel actions menu — focus return to trigger (AC-TASKS-TASK-ACTIONS-MENU-001.12)", () => {
  it("returns focus to the trigger once a cancelled Delete confirmation closes", async () => {
    renderPanel(<TaskPreviewPanel task={TASK} onClose={vi.fn()} />);

    const trigger = openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
    expect(await screen.findByRole("alertdialog")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it("returns focus to the trigger once a cancelled Detach confirmation closes", async () => {
    renderPanel(<TaskPreviewPanel task={CHILD_TASK} onClose={vi.fn()} />);

    const trigger = openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: DETACH_MENU_ITEM_NAME }));
    const popover = await screen.findByTestId(DETACH_CONFIRM_POPOVER_TEST_ID);

    fireEvent.click(within(popover).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByTestId(DETACH_CONFIRM_POPOVER_TEST_ID)).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});

describe("TaskPreviewPanel actions menu — terminal activation closes the menu (AC-TASKS-TASK-ACTIONS-MENU-004.2)", () => {
  it("closes the dropdown menu itself as soon as a terminal entry (Archive) is activated", () => {
    renderPanel(<TaskPreviewPanel task={TASK} onClose={vi.fn()} />);

    openMenu();
    expectMenuOpen(true);
    fireEvent.click(screen.getByRole("menuitem", { name: "Archive" }));

    expectMenuOpen(false);
  });

  it("closes the dropdown menu itself as soon as a terminal entry (Detach from parent) is activated", () => {
    renderPanel(<TaskPreviewPanel task={CHILD_TASK} onClose={vi.fn()} />);

    openMenu();
    expectMenuOpen(true);
    fireEvent.click(screen.getByRole("menuitem", { name: DETACH_MENU_ITEM_NAME }));

    expectMenuOpen(false);
  });
});

describe("TaskPreviewPanel actions menu — confirmation retargeting (AC-TASKS-TASK-ACTIONS-MENU-004.5a)", () => {
  const OTHER_CHILD_TASK: Task = {
    id: "task-3",
    title: "Other child task",
    workflowStepId: "step-1",
    parentTaskId: "task-other-parent",
  };

  function clickDetach() {
    openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: DETACH_MENU_ITEM_NAME }));
  }

  it("drops a detach confirmation requested for a subject the panel has since swapped away from", async () => {
    vi.useFakeTimers();
    const { rerender } = renderPanel(<TaskPreviewPanel task={CHILD_TASK} onClose={vi.fn()} />);

    clickDetach();

    // Still inside the 300ms open-delay when the subject swaps out.
    await act(async () => {
      vi.advanceTimersByTime(100);
    });
    rerenderPanel(rerender, <TaskPreviewPanel task={OTHER_CHILD_TASK} onClose={vi.fn()} />);
    await act(async () => {
      vi.advanceTimersByTime(300);
    });

    expect(screen.queryByTestId(DETACH_CONFIRM_POPOVER_TEST_ID)).toBeNull();
  });

  it("closes an already-open detach confirmation when the subject swaps away from it", async () => {
    vi.useFakeTimers();
    const { rerender } = renderPanel(<TaskPreviewPanel task={CHILD_TASK} onClose={vi.fn()} />);

    clickDetach();
    await act(async () => {
      vi.advanceTimersByTime(300);
    });
    expect(screen.getByTestId(DETACH_CONFIRM_POPOVER_TEST_ID)).toBeTruthy();

    rerenderPanel(rerender, <TaskPreviewPanel task={OTHER_CHILD_TASK} onClose={vi.fn()} />);

    expect(screen.queryByTestId(DETACH_CONFIRM_POPOVER_TEST_ID)).toBeNull();
  });
});

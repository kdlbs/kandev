import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskFlagIndicator } from "./task-flag-indicator";
import { TaskFlagEditorProvider, useOpenTaskFlag } from "./task-flag-editor-provider";
import type { TaskSwitcherItem } from "./task-switcher-types";

const saveFlag = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/use-save-task-flag", () => ({ useSaveTaskFlag: () => saveFlag }));
const viewport = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => viewport,
}));

const task: TaskSwitcherItem = {
  id: "flagged-task",
  title: "Review release notes",
  state: "IN_PROGRESS",
  flagNote: "Saved note",
};

function Flag() {
  const open = useOpenTaskFlag();
  return (
    <TaskFlagIndicator note={task.flagNote!} onOpen={(invoker) => open?.(task, invoker, true)} />
  );
}

function renderFlag() {
  render(
    <TooltipProvider>
      <TaskFlagEditorProvider>
        <Flag />
      </TaskFlagEditorProvider>
    </TooltipProvider>,
  );
  return screen.getByRole("button", { name: "Flag" });
}

afterEach(() => {
  cleanup();
  viewport.isMobile = false;
  saveFlag.mockReset();
});

it("reveals just the note when the flag receives keyboard focus", async () => {
  const flag = renderFlag();
  flag.focus();
  expect((await screen.findByRole("tooltip")).textContent).toBe(task.flagNote);
  expect(document.activeElement).toBe(flag);
  fireEvent.click(flag);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(saveFlag).not.toHaveBeenCalled();
});

it("opens the read-only flag drawer from a phone tap", async () => {
  viewport.isMobile = true;
  const flag = renderFlag();
  fireEvent.click(flag);
  const drawer = await screen.findByRole("dialog", { name: "Flag" });
  expect(drawer.textContent).toContain(task.flagNote);
  expect(screen.queryByRole("textbox")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(saveFlag).not.toHaveBeenCalled();
});

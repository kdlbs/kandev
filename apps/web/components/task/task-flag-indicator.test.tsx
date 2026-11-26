import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskFlagIndicator } from "./task-flag-indicator";
import { TaskFlagEditorProvider } from "./task-flag-editor-provider";

const saveFlag = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/use-save-task-flag", () => ({ useSaveTaskFlag: () => saveFlag }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));

afterEach(() => {
  cleanup();
  saveFlag.mockReset();
});

it("lets keyboard focus reveal the note and Tab enter the editor", async () => {
  render(
    <TooltipProvider>
      <TaskFlagEditorProvider>
        <TaskFlagIndicator taskId="flagged-task" note="Saved note" onOpen={() => {}} />
      </TaskFlagEditorProvider>
    </TooltipProvider>,
  );
  const flag = screen.getByRole("button", { name: "Flag" });
  flag.focus();
  const note = await screen.findByRole("textbox", { name: "Flag note" });
  expect(document.activeElement).toBe(flag);
  fireEvent.keyDown(flag, { key: "Tab" });
  expect(document.activeElement).toBe(note);
  fireEvent.change(note, { target: { value: "A new note" } });
  fireEvent.keyDown(note, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("textbox")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(flag));
  expect(fireEvent.keyDown(flag, { key: "Tab" })).toBe(true);
  expect(saveFlag).not.toHaveBeenCalled();
});

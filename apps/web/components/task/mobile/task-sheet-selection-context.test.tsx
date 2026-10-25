import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import {
  TaskSheetSelectionProvider,
  useTaskSheetSelectionController,
} from "./task-sheet-selection-context";
import type { TaskSheetSelectionController } from "./session-task-switcher-sheet-selection";
afterEach(cleanup);

it("invalidates pending selection when workspace scope changes without replacing the controller", () => {
  const controllers: TaskSheetSelectionController[] = [];
  function Consumer() {
    controllers.push(useTaskSheetSelectionController());
    return null;
  }
  const view = render(
    <TaskSheetSelectionProvider workspaceId="first">
      <Consumer />
    </TaskSheetSelectionProvider>,
  );
  const token = controllers[0].beginSelection();
  view.rerender(
    <TaskSheetSelectionProvider workspaceId="second">
      <Consumer />
    </TaskSheetSelectionProvider>,
  );
  expect(controllers[1]).toBe(controllers[0]);
  expect(controllers[0].isCurrent(token)).toBe(false);
});

// @covers AC-UI-MOBILE-MENU-003.3
it("shares selection cancellation between pins and the task picker until their owner unmounts", () => {
  const controllers: TaskSheetSelectionController[] = [];
  function Consumer() {
    controllers.push(useTaskSheetSelectionController());
    return null;
  }
  const view = render(
    <TaskSheetSelectionProvider>
      <Consumer />
      <Consumer />
    </TaskSheetSelectionProvider>,
  );
  expect(controllers[0]).toBe(controllers[1]);
  const first = controllers[0].beginSelection();
  const second = controllers[1].beginSelection();
  expect(controllers[0].isCurrent(first)).toBe(false);
  expect(controllers[0].isCurrent(second)).toBe(true);
  view.unmount();
  expect(controllers[0].isCurrent(second)).toBe(false);
});

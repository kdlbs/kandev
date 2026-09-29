import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";

const save = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: save }));
afterEach(cleanup);
beforeEach(() => {
  save.mockReset();
  save.mockReturnValue(new Promise(() => {}));
});

async function openColors(rowId: string, colors: Record<string, "red" | "blue" | null>) {
  const clear = vi.fn();
  render(
    <StateProvider
      initialState={{
        userSettings: {
          ...defaultState.userSettings,
          sidebarTaskColors: colors,
          revision: 1,
          loaded: true,
        },
      }}
    >
      <ToastProvider>
        <TaskItemWithContextMenu
          task={{ id: rowId, title: "Task", state: "TODO" }}
          selectedTaskIds={new Set(["a", "b"])}
          onClearSelection={clear}
        >
          <div data-testid="row">Task</div>
        </TaskItemWithContextMenu>
      </ToastProvider>
    </StateProvider>,
  );
  fireEvent.contextMenu(screen.getByTestId("row"));
  const trigger = await screen.findByRole("menuitem", { name: "Color" });
  act(() => trigger.focus());
  fireEvent.keyDown(trigger, { key: "ArrowRight" });
  await screen.findByRole("menuitemradio", { name: "Blue" });
  return clear;
}

// @covers AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.1, AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.3
it("offers bulk color without pin callbacks and keeps mixed colors unchecked", async () => {
  const clear = await openColors("a", { a: "red", b: "blue" });
  expect(
    screen
      .getAllByRole("menuitemradio")
      .every((option) => option.getAttribute("aria-checked") === "false"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("menuitemradio", { name: "Blue" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledExactlyOnceWith({
      sidebar_task_color_patch: { colors: { a: "blue", b: "blue" }, if_missing: false },
    }),
  );
  expect(clear).not.toHaveBeenCalled();
});

it("targets an unselected context row individually", async () => {
  await openColors("c", { a: "red", b: "blue" });
  fireEvent.click(screen.getByRole("menuitemradio", { name: "Blue" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledExactlyOnceWith({
      sidebar_task_color_patch: { colors: { c: "blue" }, if_missing: false },
    }),
  );
});

it("checks a unanimous color and enables clearing", async () => {
  await openColors("a", { a: "red", b: "red" });
  expect(screen.getByRole("menuitemradio", { name: "Red" }).getAttribute("aria-checked")).toBe(
    "true",
  );
  const none = screen.getByRole("menuitem", { name: "None" });
  expect(none.getAttribute("data-disabled")).toBeNull();
  fireEvent.click(none);
  await waitFor(() =>
    expect(save).toHaveBeenCalledExactlyOnceWith({
      sidebar_task_color_patch: { colors: { a: null, b: null }, if_missing: false },
    }),
  );
});

it("disables None when all manual colors are absent", async () => {
  await openColors("a", {});
  expect(
    screen.getByRole("menuitem", { name: "None" }).getAttribute("data-disabled"),
  ).not.toBeNull();
});

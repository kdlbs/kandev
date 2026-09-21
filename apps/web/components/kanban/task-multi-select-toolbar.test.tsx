import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskMultiSelectToolbar } from "./task-multi-select-toolbar";

const save = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: save }));
afterEach(() => {
  cleanup();
  save.mockReset();
});

function mount(ids: string[]) {
  const clear = vi.fn();
  render(
    <StateProvider>
      <ToastProvider>
        <TaskMultiSelectToolbar
          selectedIds={new Set(ids)}
          steps={[]}
          isProcessing={false}
          canMove={false}
          getEligibleSelectedIds={(value) => value}
          onClearSelection={clear}
          onBulkDelete={vi.fn()}
          onBulkArchive={vi.fn()}
          onBulkMove={vi.fn()}
        />
      </ToastProvider>
    </StateProvider>,
  );
  return clear;
}

it("has no color action for an empty selection", () => {
  mount([]);
  expect(screen.queryByTestId("bulk-color-button")).toBeNull();
});

// @covers AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.4, AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.5
it("colors mixed-workflow selections and exposes pending state without clearing selection", async () => {
  let resolve: ((value: unknown) => void) | undefined;
  save.mockReturnValue(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const clear = mount(["a", "b"]);
  const trigger = screen.getByTestId("bulk-color-button");
  fireEvent.keyDown(trigger, { key: "Enter" });
  fireEvent.click(await screen.findByTestId("bulk-color-option-blue"));
  await waitFor(() =>
    expect(save).toHaveBeenCalledExactlyOnceWith({
      sidebar_task_color_patch: { colors: { a: "blue", b: "blue" }, if_missing: false },
    }),
  );
  expect((trigger as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByRole("status").textContent).toBe("Saving colors...");
  expect(clear).not.toHaveBeenCalled();
  await act(async () =>
    resolve?.({
      settings: {
        user_id: "default-user",
        workspace_id: "",
        repository_ids: [],
        sidebar_task_colors: { a: "blue", b: "blue" },
        revision: 2,
      },
      shell_options: [],
    }),
  );
  expect((trigger as HTMLButtonElement).disabled).toBe(false);
});

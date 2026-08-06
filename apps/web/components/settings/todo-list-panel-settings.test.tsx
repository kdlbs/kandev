import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

const updateUserSettings = vi.fn();
const TODO_LIST_PANEL_LABEL = "Show agent todo list panel";

vi.mock("@/lib/api", () => ({
  updateUserSettings: (...args: unknown[]) => updateUserSettings(...args),
}));

import { TodoListPanelSettings } from "./todo-list-panel-settings";

beforeEach(() => {
  updateUserSettings.mockReset().mockResolvedValue({ settings: {} });
});

afterEach(cleanup);

describe("TodoListPanelSettings", () => {
  it("keeps an enabled preference local until Save changes persists it", async () => {
    render(
      <StateProvider
        initialState={{
          userSettings: { ...defaultState.userSettings, showTodoListPanel: false },
        }}
      >
        <SettingsSaveProvider>
          <TodoListPanelSettings />
        </SettingsSaveProvider>
      </StateProvider>,
    );
    const toggle = screen.getByRole("switch", { name: TODO_LIST_PANEL_LABEL });

    expect(toggle.getAttribute("data-state")).toBe("unchecked");
    fireEvent.click(toggle);

    expect(updateUserSettings).not.toHaveBeenCalled();
    expect(toggle.getAttribute("data-state")).toBe("checked");
    expect(
      screen.getByTestId("todo-list-panel-settings-card").getAttribute("data-settings-dirty"),
    ).toBe("true");

    fireEvent.click(await screen.findByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(updateUserSettings).toHaveBeenCalledWith({ show_todo_list_panel: true }),
    );
    await waitFor(() => expect(toggle.getAttribute("data-settings-dirty")).toBe("false"));
  });
});

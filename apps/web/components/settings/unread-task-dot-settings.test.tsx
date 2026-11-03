import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SettingsSaveProvider } from "./settings-save-provider";

vi.mock("@/lib/api", () => ({ updateUserSettings: vi.fn() }));

import { UnreadTaskDotSettings } from "./unread-task-dot-settings";

const DOT_LABEL = "Show unread dot on tasks";
const DELAY_LABEL = "Unread dot hide delay (seconds)";

afterEach(cleanup);

function renderSettings(unreadTaskDot: boolean) {
  render(
    <StateProvider initialState={{ userSettings: { ...defaultState.userSettings, unreadTaskDot } }}>
      <SettingsSaveProvider>
        <UnreadTaskDotSettings />
      </SettingsSaveProvider>
    </StateProvider>,
  );
}

describe("UnreadTaskDotSettings", () => {
  // @covers AC-TASKS-TASK-UNREAD-DOT-001.19
  it("offers the hide delay only while the dot switch is on", () => {
    renderSettings(false);
    expect(screen.queryByLabelText(DELAY_LABEL)).toBeNull();

    fireEvent.click(screen.getByRole("switch", { name: DOT_LABEL }));

    expect(screen.getByLabelText(DELAY_LABEL)).not.toBeNull();
    expect(
      screen.getByText(/Only changes how long an opened task's dot stays visible/),
    ).not.toBeNull();
  });

  // @covers AC-TASKS-TASK-UNREAD-DOT-001.19
  it("hides the delay again when the dot switch is turned off", () => {
    renderSettings(true);
    expect(screen.getByLabelText(DELAY_LABEL)).not.toBeNull();

    fireEvent.click(screen.getByRole("switch", { name: DOT_LABEL }));

    expect(screen.queryByLabelText(DELAY_LABEL)).toBeNull();
  });

  it("keeps an unsaved delay edit when the dot switch is turned off and on", () => {
    renderSettings(true);
    fireEvent.change(screen.getByLabelText(DELAY_LABEL), { target: { value: "7" } });
    const toggle = screen.getByRole("switch", { name: DOT_LABEL });

    fireEvent.click(toggle);
    fireEvent.click(toggle);

    expect((screen.getByLabelText(DELAY_LABEL) as HTMLInputElement).value).toBe("7");
  });
});

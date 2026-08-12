import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppStatusBarSettingsCard } from "./app-status-bar-settings-card";

afterEach(cleanup);

describe("AppStatusBarSettingsCard", () => {
  it("renders a controlled, dirty, touch-sized visibility row", () => {
    const onChange = vi.fn();
    render(<AppStatusBarSettingsCard enabled isDirty onChange={onChange} />);

    const toggle = screen.getByRole("switch", { name: "Show status bar" });
    expect(toggle.getAttribute("data-state")).toBe("checked");
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.getByTestId("app-status-bar-settings-card").dataset.settingsDirty).toBe("true");
    expect(screen.getByTestId("app-status-bar-toggle-row").className).toContain("min-h-11");
    expect(toggle.className).toContain("data-[size=default]:h-11");
    expect(toggle.className).toContain("data-[size=default]:w-11");
    expect(
      screen.getByText(
        "Show status details along the bottom of Kandev. On phones, they appear in the Status drawer. Connection warnings remain visible when this is off.",
      ),
    ).toBeTruthy();

    fireEvent.click(toggle);
    expect(onChange).toHaveBeenCalledWith(false);
    expect(onChange).toHaveBeenCalledTimes(1);
  });
});

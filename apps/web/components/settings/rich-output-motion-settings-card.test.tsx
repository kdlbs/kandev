import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RichOutputMotionSettingsCard } from "./rich-output-motion-settings-card";

afterEach(cleanup);

describe("RichOutputMotionSettingsCard", () => {
  it("renders a controlled, dirty, touch-sized device preference", () => {
    const onChange = vi.fn();
    render(<RichOutputMotionSettingsCard enabled isDirty onChange={onChange} />);

    const toggle = screen.getByRole("switch", { name: "Animate rich-output charts" });
    expect(toggle.getAttribute("data-state")).toBe("checked");
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.getByTestId("rich-output-motion-settings-card").dataset.settingsDirty).toBe(
      "true",
    );
    expect(screen.getByTestId("rich-output-motion-toggle-row").className).toContain("min-h-11");
    expect(toggle.className).toContain("data-[size=default]:h-11");
    expect(toggle.className).toContain("data-[size=default]:w-11");
    expect(
      screen.getByText(
        "Animate line and bar charts when agents present them. Turn this off to reduce browser work; chart data and controls stay available. Your operating system's reduced-motion setting always takes priority. Saved on this device.",
      ),
    ).toBeTruthy();

    fireEvent.click(toggle);
    expect(onChange).toHaveBeenCalledWith(false);
    expect(onChange).toHaveBeenCalledOnce();
  });
});

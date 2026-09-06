import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SidebarSettingsDisclosure } from "./sidebar-settings-disclosure";

afterEach(cleanup);

describe("SidebarSettingsDisclosure", () => {
  it("toggles uncontrolled content without changing the surrounding draft", () => {
    render(
      <SidebarSettingsDisclosure title="Settings" testId="settings">
        <span>Content</span>
      </SidebarSettingsDisclosure>,
    );

    expect(screen.queryByText("Content")).toBeNull();
    fireEvent.click(screen.getByTestId("settings-toggle"));
    expect(screen.getByText("Content")).toBeTruthy();
    expect(screen.getByTestId("settings-toggle").getAttribute("aria-expanded")).toBe("true");
  });

  it("reports changes while leaving controlled expansion to the parent", () => {
    const onExpandedChange = vi.fn();
    render(
      <SidebarSettingsDisclosure
        title="Settings"
        testId="settings"
        expanded={false}
        onExpandedChange={onExpandedChange}
      >
        <span>Content</span>
      </SidebarSettingsDisclosure>,
    );

    fireEvent.click(screen.getByTestId("settings-toggle"));
    expect(onExpandedChange).toHaveBeenCalledWith(true);
    expect(screen.queryByText("Content")).toBeNull();
  });
});

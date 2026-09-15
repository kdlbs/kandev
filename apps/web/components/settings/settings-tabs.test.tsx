import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import {
  SettingsTabs,
  SettingsTabsList,
  SettingsTabsPanel,
  type SettingsTabOption,
} from "./settings-tabs";

const tabs: SettingsTabOption[] = [
  { id: "database", label: "Database" },
  { id: "logs", label: "Logs" },
];

afterEach(cleanup);

function ExampleTabs() {
  const [value, setValue] = useState("database");
  return (
    <SettingsTabs tabs={tabs} value={value} onValueChange={setValue}>
      <SettingsTabsList ariaLabel="Data and logs" />
      <SettingsTabsPanel value="database" testId="database-panel">
        Database content
      </SettingsTabsPanel>
      <SettingsTabsPanel value="logs" testId="logs-panel">
        Logs content
      </SettingsTabsPanel>
    </SettingsTabs>
  );
}

describe("SettingsTabs", () => {
  it("connects header triggers to panels and uses manual activation", () => {
    render(<ExampleTabs />);

    const database = screen.getByRole("tab", { name: "Database" });
    const logs = screen.getByRole("tab", { name: "Logs" });
    expect(database.getAttribute("aria-controls")).toBeTruthy();
    expect(logs.getAttribute("aria-controls")).toBeTruthy();
    expect(database.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByTestId("database-panel").getAttribute("data-state")).toBe("active");
    expect(screen.getByTestId("logs-panel").getAttribute("data-state")).toBe("inactive");

    fireEvent.keyDown(database, { key: "ArrowRight" });
    expect(database.getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(logs, { key: "Enter" });
    expect(logs.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByTestId("logs-panel").getAttribute("data-state")).toBe("active");
  });

  it("keeps desktop and touch target sizing on the shared controls", () => {
    render(<ExampleTabs />);

    expect(screen.getAllByRole("tablist")[0]?.className).toContain("h-8");
    expect(screen.getByRole("tab", { name: "Database" }).className).toContain("h-7");
    expect(screen.getByRole("tab", { name: "Database" }).className).toContain("h-11");
  });
});

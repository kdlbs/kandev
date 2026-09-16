import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import {
  SettingsTabs,
  SettingsTabsList,
  SettingsTabsPanel,
  type SettingsTabOption,
} from "./settings-tabs";

const databaseTabLabel = "Database";
const logsTabLabel = "Logs";
const databasePanelTestId = "database-panel";
const logsPanelTestId = "logs-panel";

const tabs: SettingsTabOption[] = [
  { id: "database", label: databaseTabLabel },
  { id: "logs", label: logsTabLabel },
];

afterEach(cleanup);

function ExampleTabs() {
  const [value, setValue] = useState("database");
  return (
    <SettingsTabs tabs={tabs} value={value} onValueChange={setValue}>
      <SettingsTabsList ariaLabel="Data and logs" />
      <SettingsTabsPanel value="database" testId={databasePanelTestId}>
        Database content
      </SettingsTabsPanel>
      <SettingsTabsPanel value="logs" testId={logsPanelTestId}>
        Logs content
      </SettingsTabsPanel>
    </SettingsTabs>
  );
}

describe("SettingsTabs", () => {
  it("connects header triggers to panels and uses manual activation", () => {
    render(<ExampleTabs />);

    const database = screen.getByRole("tab", { name: databaseTabLabel });
    const logs = screen.getByRole("tab", { name: logsTabLabel });
    expect(database.getAttribute("aria-controls")).toBeTruthy();
    expect(logs.getAttribute("aria-controls")).toBeTruthy();
    expect(database.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByTestId(databasePanelTestId).getAttribute("data-state")).toBe("active");
    expect(screen.getByTestId(logsPanelTestId).getAttribute("data-state")).toBe("inactive");
    expect(screen.getByTestId(logsPanelTestId).textContent).toBe("");

    fireEvent.keyDown(database, { key: "ArrowRight" });
    expect(database.getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(logs, { key: "Enter" });
    expect(logs.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByTestId(logsPanelTestId).getAttribute("data-state")).toBe("active");
    expect(screen.getByTestId(logsPanelTestId).textContent).toContain("Logs content");
  });

  it("retains visited panel content when selection changes", () => {
    render(<ExampleTabs />);
    fireEvent.keyDown(screen.getByRole("tab", { name: logsTabLabel }), { key: "Enter" });
    expect(screen.getByTestId(logsPanelTestId).textContent).toContain("Logs content");
    fireEvent.keyDown(screen.getByRole("tab", { name: databaseTabLabel }), { key: "Enter" });
    expect(screen.getByTestId(logsPanelTestId).getAttribute("aria-hidden")).toBe("true");
    expect(screen.getByTestId(logsPanelTestId).textContent).toContain("Logs content");
    expect(screen.getByTestId(databasePanelTestId).getAttribute("aria-hidden")).toBe("false");
  });
});

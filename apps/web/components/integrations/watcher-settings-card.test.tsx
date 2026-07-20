import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { WatcherSettingsCard } from "./watcher-settings-card";

describe("WatcherSettingsCard", () => {
  it("marks the owning card dirty", () => {
    render(
      <WatcherSettingsCard isDirty isLoading={false} isEmpty={false} testId="watchers-card">
        <div>Rows</div>
      </WatcherSettingsCard>,
    );

    expect(screen.getByTestId("watchers-card").getAttribute("data-settings-dirty")).toBe("true");
  });
});

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SettingsCardHeader } from "./settings-card-header";

describe("SettingsCardHeader", () => {
  it("renders a semantic card heading and an action slot", () => {
    render(
      <SettingsCardHeader
        title={<span data-testid="settings-card-title-slot" />}
        actions={<div data-testid="settings-card-action-slot" />}
      />,
    );

    expect(
      screen
        .getByRole("heading", { level: 3 })
        .contains(screen.getByTestId("settings-card-title-slot")),
    ).toBe(true);
    expect(screen.getByTestId("settings-card-action-slot")).toBeTruthy();
  });
});

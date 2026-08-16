import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SettingsField } from "./settings-field";

describe("SettingsField", () => {
  it("keeps the label, helper, and error roles together", () => {
    render(
      <SettingsField
        label={<span data-testid="field-label" />}
        helper={<span data-testid="field-helper" />}
        error={<span data-testid="field-error" />}
      >
        <input aria-label="field-control" />
      </SettingsField>,
    );

    expect(screen.getByTestId("field-label")).toBeTruthy();
    expect(screen.getByTestId("field-helper")).toBeTruthy();
    expect(screen.getByRole("alert").contains(screen.getByTestId("field-error"))).toBe(true);
    expect(screen.getByRole("textbox", { name: "field-control" })).toBeTruthy();
  });
});

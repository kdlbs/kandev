import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { PropertyRow } from "./property-row";

afterEach(() => cleanup());

describe("PropertyRow", () => {
  it("renders the label and child value", () => {
    const { getByText } = render(
      <PropertyRow label="Status">
        <span>In Progress</span>
      </PropertyRow>,
    );
    expect(getByText("Status")).toBeTruthy();
    expect(getByText("In Progress")).toBeTruthy();
  });

  it("centres children by default", () => {
    const { container } = render(
      <PropertyRow label="L">
        <span>v</span>
      </PropertyRow>,
    );
    const row = container.firstChild as HTMLElement;
    expect(row.className).toMatch(/items-center/);
    expect(row.className).not.toMatch(/items-start/);
  });

  it("aligns children to top when alignStart=true", () => {
    const { container } = render(
      <PropertyRow label="L" alignStart>
        <span>v</span>
      </PropertyRow>,
    );
    const row = container.firstChild as HTMLElement;
    expect(row.className).toMatch(/items-start/);
  });

  it("applies valueClassName to the value side", () => {
    const { container } = render(
      <PropertyRow label="L" valueClassName="custom-value">
        <span>v</span>
      </PropertyRow>,
    );
    const value = container.querySelector(".custom-value");
    expect(value).not.toBeNull();
  });
});

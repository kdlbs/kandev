import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ModeCombobox } from "./mode-combobox";
import type { ModeEntry } from "@/lib/types/http";

afterEach(cleanup);

const TRIGGER_TEST_ID = "profile-mode-select";
const CUSTOM_ENTRY_TEST_ID = "profile-mode-custom-entry";
const SEARCH_PLACEHOLDER = "Search modes...";
const CUSTOM_MODE_ID = "reviewer";

const modes: ModeEntry[] = [
  { id: "build", name: "Build" },
  { id: "plan", name: "Plan" },
];

function renderCombobox(
  value: string,
  onChange: (value: string) => void = vi.fn(),
  currentModeId: string | undefined = "build",
) {
  return render(
    <ModeCombobox value={value} onChange={onChange} modes={modes} currentModeId={currentModeId} />,
  );
}

function openCombobox() {
  fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));
}

describe("ModeCombobox", () => {
  it("shows a stored mode that is not advertised as a selectable custom row", () => {
    const onChange = vi.fn();
    renderCombobox("planner", onChange);

    openCombobox();

    const row = screen.getByTestId("profile-mode-custom-row");
    expect(row.textContent).toContain("planner");
    expect(row.textContent).toContain("Custom");

    fireEvent.click(row);
    expect(onChange).toHaveBeenCalledWith("planner");
  });

  it("offers a custom entry for typed text with no exact match and selects it verbatim", () => {
    const onChange = vi.fn();
    renderCombobox("build", onChange);

    openCombobox();
    fireEvent.change(screen.getByPlaceholderText(SEARCH_PLACEHOLDER), {
      target: { value: CUSTOM_MODE_ID },
    });

    const entry = screen.getByTestId(CUSTOM_ENTRY_TEST_ID);
    expect(entry.textContent).toContain(`Use "${CUSTOM_MODE_ID}" as mode ID`);

    fireEvent.click(entry);
    expect(onChange).toHaveBeenCalledWith(CUSTOM_MODE_ID);
  });

  it("does not offer the custom entry once the typed text exactly matches a listed mode", () => {
    renderCombobox("build");

    openCombobox();
    fireEvent.change(screen.getByPlaceholderText(SEARCH_PLACEHOLDER), {
      target: { value: "plan" },
    });

    expect(screen.queryByTestId(CUSTOM_ENTRY_TEST_ID)).toBeNull();
  });

  it("activates the typed entry over an advertised match on Enter", () => {
    const onChange = vi.fn();
    renderCombobox("build", onChange);

    openCombobox();
    const input = screen.getByPlaceholderText(SEARCH_PLACEHOLDER);
    fireEvent.change(input, { target: { value: "pla" } });

    expect(screen.getByTestId(CUSTOM_ENTRY_TEST_ID).textContent).toContain('Use "pla" as mode ID');
    expect(screen.getByText("Plan")).toBeTruthy();

    fireEvent.keyDown(input, { key: "Enter" });
    expect(onChange).toHaveBeenCalledWith("pla");
  });

  it("clears the search filter after selecting a mode before reopening", () => {
    const onChange = vi.fn();
    renderCombobox("build", onChange);

    openCombobox();
    fireEvent.change(screen.getByPlaceholderText(SEARCH_PLACEHOLDER), {
      target: { value: CUSTOM_MODE_ID },
    });
    expect(screen.queryByRole("option", { name: /^Build/ })).toBeNull();

    fireEvent.click(screen.getByTestId(CUSTOM_ENTRY_TEST_ID));
    expect(onChange).toHaveBeenCalledWith(CUSTOM_MODE_ID);
    fireEvent.click(screen.getByTestId(TRIGGER_TEST_ID));

    const reopenedInput = screen.getByPlaceholderText(SEARCH_PLACEHOLDER) as HTMLInputElement;
    expect(reopenedInput.value).toBe("");
    expect(screen.getByRole("option", { name: /^Build/ })).toBeTruthy();
    expect(screen.getByRole("option", { name: /^Plan/ })).toBeTruthy();
  });

  it("shows the raw id in the trigger when the stored mode is not advertised", () => {
    renderCombobox("planner");
    expect(screen.getByTestId(TRIGGER_TEST_ID).textContent).toContain("planner");
  });
});

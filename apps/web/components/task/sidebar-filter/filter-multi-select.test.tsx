import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { FilterMultiSelect, type MultiSelectOption } from "./filter-multi-select";

afterEach(cleanup);

const OPTION_TEST_ID = "filter-value-multi-option";

const options = [
  { value: "a", label: "Repository A" },
  { value: "b", label: "Repository B" },
  { value: "c", label: "Repository C" },
  { value: "d", label: "Repository D" },
];

function Harness({
  items = options,
  initial = ["d", "b", "missing"],
  onChange = vi.fn(),
}: {
  items?: MultiSelectOption[];
  initial?: string[];
  onChange?: (next: string[]) => void;
}) {
  const [selected, setSelected] = useState(initial);
  return (
    <FilterMultiSelect
      options={items}
      selected={selected}
      onChange={(next) => {
        onChange(next);
        setSelected(next);
      }}
    />
  );
}

function values() {
  return screen.queryAllByTestId(OPTION_TEST_ID).map((row) => row.dataset.value?.split(" ").at(-1));
}

async function openPicker() {
  fireEvent.click(screen.getByTestId("filter-value-multi"));
  await screen.findByRole("combobox");
}

describe("FilterMultiSelect", () => {
  // @covers AC-UI-FILTER-SELECTED-FIRST-001.1
  it("opens with available selections first without changing membership", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await openPicker();
    await waitFor(() => expect(values()).toEqual(["b", "d", "a", "c"]));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getAllByTestId(OPTION_TEST_ID)[0].dataset.checked).toBe("true");
  });

  // @covers AC-UI-FILTER-SELECTED-FIRST-001.2
  it("places selections from every workflow before unselected steps with group context", async () => {
    render(
      <Harness
        items={[
          { value: "a", label: "Ready", group: "Alpha" },
          { value: "b", label: "Done", group: "Alpha" },
          { value: "c", label: "Done", group: "Beta" },
          { value: "d", label: "Review", group: "Beta" },
        ]}
      />,
    );
    await openPicker();
    await waitFor(() => expect(values()).toEqual(["b", "d", "a", "c"]));
    const rows = screen.getAllByTestId(OPTION_TEST_ID);
    expect(rows.map((row) => row.closest("[cmdk-group]")?.textContent)).toEqual([
      "AlphaDone",
      "BetaReview",
      "AlphaReady",
      "BetaDone",
    ]);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "Beta Done" } });
    await waitFor(() => expect(values()).toEqual(["c"]));
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "" } });
    await waitFor(() => expect(values()).toEqual(["b", "d", "a", "c"]));
  });

  // @covers AC-UI-FILTER-SELECTED-FIRST-001.3
  it("reorders controlled toggles while retaining missing selected values", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await openPicker();
    fireEvent.click(
      screen.getAllByTestId(OPTION_TEST_ID).find((row) => row.dataset.value?.endsWith(" a"))!,
    );
    await waitFor(() => expect(values()).toEqual(["a", "b", "d", "c"]));
    expect(onChange).toHaveBeenLastCalledWith(["d", "b", "missing", "a"]);
    expect(screen.getByRole("combobox")).toBeTruthy();
  });

  // @covers AC-UI-FILTER-SELECTED-FIRST-001.4
  it("filters selected nonmatches and restores selected-first order after clearing or reopening", async () => {
    render(<Harness />);
    await openPicker();
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "Repository" } });
    await waitFor(() => expect(values()).toEqual(["a", "b", "c", "d"]));
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "Repository C" } });
    await waitFor(() => expect(values()).toEqual(["c"]));
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "no such repository" } });
    await waitFor(() => expect(values()).toEqual([]));
    expect(screen.getByText("No options.")).toBeTruthy();
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "" } });
    await waitFor(() => expect(values()).toEqual(["b", "d", "a", "c"]));
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "Repository C" } });
    fireEvent.keyDown(screen.getByRole("combobox"), { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("combobox")).toBeNull());
    await openPicker();
    await waitFor(() => expect(values()).toEqual(["b", "d", "a", "c"]));
  });

  // @covers AC-UI-FILTER-SELECTED-FIRST-001.3
  // @covers AC-UI-FILTER-SELECTED-FIRST-001.5
  it("supports consecutive keyboard toggles across partition moves and returns focus on dismissal", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await openPicker();
    const input = screen.getByRole("combobox");
    fireEvent.keyDown(input, { key: "Home" });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(["d", "missing"]));
    expect(document.activeElement).toBe(input);
    fireEvent.keyDown(input, { key: "End" });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(["d", "missing", "c"]));
    fireEvent.keyDown(input, { key: "Escape" });
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId("filter-value-multi")),
    );
  });
});

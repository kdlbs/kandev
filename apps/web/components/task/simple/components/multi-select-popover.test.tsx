import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MultiSelectPopover, type MultiSelectItem } from "./multi-select-popover";

afterEach(() => cleanup());

type Item = MultiSelectItem;

const ITEMS: Item[] = [
  { id: "a", label: "Alpha", keywords: ["alpha"] },
  { id: "b", label: "Beta", keywords: ["beta"] },
  { id: "c", label: "Gamma", keywords: ["gamma"] },
];

function renderHarness(props: {
  selectedIds: string[];
  onAdd?: (id: string) => void | Promise<void>;
  onRemove?: (id: string) => void | Promise<void>;
}) {
  const onAdd = props.onAdd ?? vi.fn();
  const onRemove = props.onRemove ?? vi.fn();
  render(
    <MultiSelectPopover
      items={ITEMS}
      selectedIds={props.selectedIds}
      onAdd={onAdd}
      onRemove={onRemove}
      renderChip={(it, remove) => (
        <span key={it.id} data-testid={`chip-${it.id}`}>
          {it.label}
          <span role="button" tabIndex={0} onClick={remove} aria-label={`x-${it.id}`}>
            x
          </span>
        </span>
      )}
      renderItem={(it) => <span>{it.label}</span>}
      addLabel="+ Add"
      testId="ms-trigger"
    />,
  );
  return { onAdd, onRemove };
}

describe("MultiSelectPopover", () => {
  it("renders the empty addLabel when no items are selected", () => {
    renderHarness({ selectedIds: [] });
    expect(screen.getByTestId("ms-trigger").textContent).toContain("+ Add");
  });

  it("renders chips for selected items", () => {
    renderHarness({ selectedIds: ["a", "b"] });
    expect(screen.getByTestId("chip-a")).toBeTruthy();
    expect(screen.getByTestId("chip-b")).toBeTruthy();
  });

  it("calls onAdd when a non-selected option is chosen in the popover", async () => {
    const onAdd = vi.fn();
    renderHarness({ selectedIds: [], onAdd });
    fireEvent.click(screen.getByTestId("ms-trigger"));
    const option = await screen.findByTestId("multi-select-add-a");
    await act(async () => {
      fireEvent.click(option);
    });
    expect(onAdd).toHaveBeenCalledWith("a");
  });

  it("calls onRemove when a selected entry is clicked again", async () => {
    const onRemove = vi.fn();
    renderHarness({ selectedIds: ["a"], onRemove });
    fireEvent.click(screen.getByTestId("ms-trigger"));
    const item = await screen.findByTestId("multi-select-remove-a");
    await act(async () => {
      fireEvent.click(item);
    });
    expect(onRemove).toHaveBeenCalledWith("a");
  });
});

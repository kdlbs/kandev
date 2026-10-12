import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SortSpec } from "@/lib/state/slices/ui/sidebar-view-types";
import { SortChainEditor } from "./sort-chain-editor";

afterEach(cleanup);

const chain: SortSpec = {
  key: "running",
  direction: "desc",
  thenBy: [{ key: "lastActivityAt", direction: "desc" }],
};

it("adds, reorders, and removes individual desktop rules", () => {
  const onChange = vi.fn();
  const { rerender } = render(
    <SortChainEditor value={chain} onChange={onChange} isDrawerLayout={false} />,
  );

  fireEvent.click(screen.getByTestId("sort-add-rule-button"));
  const added: SortSpec = {
    key: "running",
    direction: "desc",
    thenBy: [
      { key: "lastActivityAt", direction: "desc" },
      { key: "state", direction: "asc" },
    ],
  };
  expect(onChange).toHaveBeenLastCalledWith(added);
  rerender(<SortChainEditor value={added} onChange={onChange} isDrawerLayout={false} />);

  fireEvent.pointerDown(screen.getByTestId("sort-rule-more-2"), {
    button: 0,
    pointerType: "mouse",
  });
  expect(onChange).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByTestId("sort-rule-more-2-move-up"));
  const reordered: SortSpec = {
    key: "running",
    direction: "desc",
    thenBy: [
      { key: "state", direction: "asc" },
      { key: "lastActivityAt", direction: "desc" },
    ],
  };
  expect(onChange).toHaveBeenLastCalledWith(reordered);
  rerender(<SortChainEditor value={reordered} onChange={onChange} isDrawerLayout={false} />);

  fireEvent.click(screen.getByTestId("sort-rule-remove-1"));
  expect(onChange).toHaveBeenLastCalledWith({
    key: "running",
    direction: "desc",
    thenBy: [{ key: "lastActivityAt", direction: "desc" }],
  });
});

it("keeps sort reordering and removal unavailable for a single rule", () => {
  render(
    <SortChainEditor
      value={{ key: "running", direction: "desc" }}
      onChange={vi.fn()}
      isDrawerLayout={false}
    />,
  );

  fireEvent.pointerDown(screen.getByTestId("sort-rule-more-0"), {
    button: 0,
    pointerType: "mouse",
  });

  expect(screen.getByTestId("sort-rule-more-0-move-up").getAttribute("aria-disabled")).toBe("true");
  expect(screen.getByTestId("sort-rule-more-0-move-down").getAttribute("aria-disabled")).toBe(
    "true",
  );
  expect((screen.getByTestId("sort-rule-remove-0") as HTMLButtonElement).disabled).toBe(true);
});

it("keeps Custom standalone and stacks phone cards with touch-sized controls", () => {
  const onChange = vi.fn();
  const { container, rerender } = render(
    <SortChainEditor
      value={{ key: "running", direction: "desc" }}
      onChange={onChange}
      isDrawerLayout
    />,
  );
  fireEvent.click(screen.getByTestId("sort-key-select"));
  fireEvent.click(screen.getByRole("option", { name: "Custom" }));

  expect(onChange).toHaveBeenCalledWith({ key: "custom", direction: "asc" });
  rerender(
    <SortChainEditor
      value={{ key: "custom", direction: "asc" }}
      onChange={onChange}
      isDrawerLayout
    />,
  );
  expect(container.querySelector('[data-testid="sort-rule-card-0"]')?.className).toContain(
    "flex-col",
  );
  for (const control of ["sort-rule-more-0", "sort-rule-remove-0"]) {
    expect(screen.getByTestId(control).className).toContain("size-11");
  }
  expect(screen.getByTestId("sort-add-rule-button").hasAttribute("disabled")).toBe(true);
});

it("selects labeled color rules without allowing duplicate preferred colors", () => {
  const onChange = vi.fn();
  const view: SortSpec = {
    key: "state",
    direction: "asc",
    thenBy: [{ key: "updatedAt", direction: "desc" }],
  };
  const { rerender } = render(
    <SortChainEditor value={view} onChange={onChange} isDrawerLayout={false} />,
  );
  fireEvent.click(screen.getByTestId("sort-key-select"));
  fireEvent.click(screen.getByRole("option", { name: "Color" }));
  const gray: SortSpec = {
    key: "color",
    color: "gray",
    direction: "desc",
    thenBy: [{ key: "updatedAt", direction: "desc" }],
  };
  expect(onChange).toHaveBeenLastCalledWith(gray);
  rerender(<SortChainEditor value={gray} onChange={onChange} isDrawerLayout={false} />);
  fireEvent.click(screen.getByTestId("sort-rule-color-0"));
  fireEvent.click(screen.getByRole("option", { name: "Blue" }));

  const blue: SortSpec = {
    key: "color",
    color: "blue",
    direction: "desc",
    thenBy: [{ key: "updatedAt", direction: "desc" }],
  };
  expect(onChange).toHaveBeenLastCalledWith(blue);
  rerender(<SortChainEditor value={blue} onChange={onChange} isDrawerLayout={false} />);
  fireEvent.click(screen.getByTestId("sort-rule-key-1"));
  fireEvent.click(screen.getByRole("option", { name: "Color" }));

  const twoColors: SortSpec = {
    key: "color",
    color: "blue",
    direction: "desc",
    thenBy: [{ key: "color", color: "gray", direction: "desc" }],
  };
  expect(onChange).toHaveBeenLastCalledWith(twoColors);
  rerender(<SortChainEditor value={twoColors} onChange={onChange} isDrawerLayout={false} />);
  fireEvent.click(screen.getByTestId("sort-rule-color-1"));
  expect(screen.queryByRole("option", { name: "Blue" })).toBeNull();
  expect(screen.getByRole("option", { name: "Gray" })).toBeTruthy();
});

it("explains when unsupported stored rules were removed", () => {
  render(
    <SortChainEditor value={chain} onChange={vi.fn()} isDrawerLayout={false} warningCount={1} />,
  );
  expect(
    screen.getByText("Some unsupported sort rules were removed. Review the sort fields.")
      .textContent,
  ).toContain("Some unsupported sort rules were removed");
});

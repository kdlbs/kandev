import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ColumnsMenu } from "./columns-menu";

afterEach(cleanup);

const WF = { id: "wf-a", name: "Workflow A" };
const ARIA_CHECKED = "aria-checked";
const STEP_A_ITEM = "columns-menu-step-step-a";
const STEP_Z_ITEM = "columns-menu-step-step-z";

function baseProps(overrides: Partial<React.ComponentProps<typeof ColumnsMenu>> = {}) {
  return {
    workflowId: WF.id,
    workflowName: WF.name,
    steps: [
      { id: "step-a", title: "A step", position: 0 },
      { id: "step-z", title: "Z step", position: 1 },
    ],
    hiddenStepIds: [] as string[],
    onToggle: vi.fn(),
    autoHideEmpty: false,
    onToggleAutoHide: vi.fn(),
    ...overrides,
  };
}

function openMenu(workflowId = WF.id) {
  const trigger = screen.getByTestId(`columns-menu-${workflowId}`);
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.pointerUp(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.click(trigger);
}

describe("ColumnsMenu — one workflow's steps", () => {
  it("renders a trigger per workflow and no group container", () => {
    render(<ColumnsMenu {...baseProps()} />);

    expect(screen.getByTestId(`columns-menu-${WF.id}`)).toBeTruthy();
    // The relocation deletes grouping entirely: a lane menu has no group
    // header, no disclosure control, and no shown-count summary.
    expect(screen.queryByTestId(`steps-filter-group-${WF.id}`)).toBeNull();
    expect(screen.queryByTestId(`steps-filter-group-toggle-${WF.id}`)).toBeNull();
  });

  it("orders steps by ascending position, tiebreaking by ascending id", () => {
    render(
      <ColumnsMenu
        {...baseProps({
          steps: [
            { id: "step-z", title: "Z step", position: 1 },
            { id: "step-a", title: "A step", position: 0 },
            { id: "step-b-tie", title: "B tie", position: 1 },
          ],
        })}
      />,
    );
    openMenu();

    const items = Array.from(document.querySelectorAll('[data-testid^="columns-menu-step-"]'));
    expect(items.map((item) => item.textContent)).toEqual(["A step", "B tie", "Z step"]);
  });

  it("ticks every step by default and unticks only the hidden ones", () => {
    render(<ColumnsMenu {...baseProps({ hiddenStepIds: ["step-z"] })} />);
    openMenu();

    expect(screen.getByTestId(STEP_A_ITEM).getAttribute(ARIA_CHECKED)).toBe("true");
    expect(screen.getByTestId(STEP_Z_ITEM).getAttribute(ARIA_CHECKED)).toBe("false");
  });

  it("reports the workflow id and step id when an item is toggled", () => {
    const onToggle = vi.fn();
    render(<ColumnsMenu {...baseProps({ onToggle })} />);
    openMenu();

    fireEvent.click(screen.getByTestId(STEP_Z_ITEM));

    expect(onToggle).toHaveBeenCalledExactlyOnceWith(WF.id, "step-z");
  });

  it("reports the workflow when automatic empty-column hiding is toggled", () => {
    const onToggleAutoHide = vi.fn();
    render(<ColumnsMenu {...baseProps({ onToggleAutoHide })} />);
    openMenu();

    fireEvent.click(screen.getByTestId(`columns-menu-auto-hide-empty-${WF.id}`));

    expect(onToggleAutoHide).toHaveBeenCalledExactlyOnceWith(WF.id);
  });

  it("separates display behavior from individual column visibility", () => {
    render(<ColumnsMenu {...baseProps()} />);
    openMenu();

    const toggle = screen.getByTestId(`columns-menu-auto-hide-empty-${WF.id}`);
    const menu = toggle.closest<HTMLElement>('[role="menu"]');
    expect(menu).not.toBeNull();
    const menuQueries = within(menu!);
    const label = menuQueries.getByText("Auto-hide empty columns");
    expect(label.classList.contains("whitespace-normal")).toBe(true);
    expect(label.classList.contains("truncate")).toBe(false);
    expect(menuQueries.queryByText("Show empty steps again while moving tasks.")).toBeNull();
    expect(
      Array.from(menu!.querySelectorAll('[data-slot="dropdown-menu-label"]')).map(
        (element) => element.textContent,
      ),
    ).toEqual(["Display Options", "Columns"]);
    expect(menu!.querySelectorAll('[data-slot="dropdown-menu-separator"]')).toHaveLength(1);
  });

  it("ignores a hidden id that no longer matches a live step", () => {
    render(<ColumnsMenu {...baseProps({ hiddenStepIds: ["step-deleted"] })} />);
    openMenu();

    expect(document.querySelectorAll('[data-testid^="columns-menu-step-"]')).toHaveLength(2);
    expect(screen.getByTestId(STEP_A_ITEM).getAttribute(ARIA_CHECKED)).toBe("true");
    expect(screen.getByTestId(STEP_Z_ITEM).getAttribute(ARIA_CHECKED)).toBe("true");
  });

  it("renders a trigger with no items for a workflow that has no steps", () => {
    render(<ColumnsMenu {...baseProps({ steps: [] })} />);

    expect(screen.getByTestId(`columns-menu-${WF.id}`)).toBeTruthy();
    openMenu();
    expect(document.querySelectorAll('[data-testid^="columns-menu-step-"]')).toHaveLength(0);
  });

  it("names the workflow in the trigger's accessible label", () => {
    render(<ColumnsMenu {...baseProps()} />);

    expect(screen.getByTestId(`columns-menu-${WF.id}`).getAttribute("aria-label")).toContain(
      WF.name,
    );
  });
});

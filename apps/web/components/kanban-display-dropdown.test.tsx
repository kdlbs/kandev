import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KanbanDisplayDropdown } from "./kanban-display-dropdown";

vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({
    workflows: [],
    activeWorkflowId: null,
    repositories: [],
    repositoriesLoading: false,
    allRepositoriesSelected: true,
    selectedRepositoryId: null,
    enablePreviewOnClick: false,
    tasksListShowDetails: false,
    onWorkflowChange: vi.fn(),
    onRepositoryChange: vi.fn(),
    onTogglePreviewOnClick: vi.fn(),
    onToggleTasksListShowDetails: vi.fn(),
  }),
}));

afterEach(cleanup);

describe("KanbanDisplayDropdown — plugin task filters", () => {
  function openDropdown() {
    const trigger = screen.getByTestId("display-button");
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
    fireEvent.pointerUp(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
    fireEvent.click(trigger);
  }

  it("renders no plugin filter section when none are registered", () => {
    render(<KanbanDisplayDropdown pluginFilters={[]} />);
    openDropdown();

    expect(screen.queryByTestId(/display-plugin-filter-/)).toBeNull();
  });

  it("renders a filter section with its options and current selection", () => {
    render(
      <KanbanDisplayDropdown
        pluginFilters={[
          {
            pluginId: "kandev-plugin-tags",
            id: "tags",
            label: "Tags",
            getOptions: () => [
              { value: "bug", label: "Bug" },
              { value: "feature", label: "Feature" },
            ],
            matches: () => true,
          },
        ]}
        pluginFilterSelections={{ "kandev-plugin-tags:tags": ["bug"] }}
      />,
    );
    openDropdown();

    expect(screen.getByTestId("display-plugin-filter-tags")).not.toBeNull();
    expect(screen.getByText("Tags")).not.toBeNull();
    expect(
      screen.getByTestId("display-plugin-filter-tags-option-bug").getAttribute("data-state"),
    ).toBe("checked");
    expect(
      screen.getByTestId("display-plugin-filter-tags-option-feature").getAttribute("data-state"),
    ).toBe("unchecked");
  });

  it("invokes onPluginFilterChange with the updated selection when toggling an option", () => {
    const onPluginFilterChange = vi.fn();
    render(
      <KanbanDisplayDropdown
        pluginFilters={[
          {
            pluginId: "kandev-plugin-tags",
            id: "tags",
            label: "Tags",
            getOptions: () => [{ value: "bug", label: "Bug" }],
            matches: () => true,
          },
        ]}
        pluginFilterSelections={{}}
        onPluginFilterChange={onPluginFilterChange}
      />,
    );
    openDropdown();

    fireEvent.click(screen.getByTestId("display-plugin-filter-tags-option-bug"));

    expect(onPluginFilterChange).toHaveBeenCalledWith("kandev-plugin-tags:tags", ["bug"]);
  });
});

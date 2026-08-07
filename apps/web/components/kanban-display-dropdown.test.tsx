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

const TAGS_PLUGIN_ID = "kandev-plugin-tags";
const TAGS_FILTER_ID = "tags";
const TAGS_FILTER_KEY = `${TAGS_PLUGIN_ID}:${TAGS_FILTER_ID}`;
const TAGS_FILTER_TEST_ID = `display-plugin-filter-${TAGS_FILTER_KEY}`;
const PRIORITY_PLUGIN_ID = "kandev-plugin-priority";
const PRIORITY_FILTER_KEY = `${PRIORITY_PLUGIN_ID}:${TAGS_FILTER_ID}`;
const DATA_STATE_ATTRIBUTE = "data-state";

const TAGS_FILTER = {
  pluginId: TAGS_PLUGIN_ID,
  id: TAGS_FILTER_ID,
  label: "Tags",
  getOptions: () => [
    { value: "bug", label: "Bug" },
    { value: "feature", label: "Feature" },
  ],
  matches: () => true,
};

function openDropdown() {
  const trigger = screen.getByTestId("display-button");
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.pointerUp(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.click(trigger);
}

describe("KanbanDisplayDropdown — plugin task filters", () => {
  it("renders no plugin filter section when none are registered", () => {
    render(<KanbanDisplayDropdown pluginFilters={[]} />);
    openDropdown();

    expect(screen.queryByTestId(/display-plugin-filter-/)).toBeNull();
  });

  it("renders a filter section with its options and current selection", () => {
    render(
      <KanbanDisplayDropdown
        pluginFilters={[TAGS_FILTER]}
        pluginFilterSelections={{ [TAGS_FILTER_KEY]: ["bug"] }}
      />,
    );
    openDropdown();

    expect(screen.getByTestId(TAGS_FILTER_TEST_ID)).not.toBeNull();
    expect(screen.getByText("Tags")).not.toBeNull();
    expect(
      screen.getByTestId(`${TAGS_FILTER_TEST_ID}-option-bug`).getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe("checked");
    expect(
      screen
        .getByTestId(`${TAGS_FILTER_TEST_ID}-option-feature`)
        .getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe("unchecked");
  });

  it("invokes onPluginFilterChange with the updated selection when toggling an option", () => {
    const onPluginFilterChange = vi.fn();
    render(
      <KanbanDisplayDropdown
        pluginFilters={[TAGS_FILTER]}
        pluginFilterSelections={{}}
        onPluginFilterChange={onPluginFilterChange}
      />,
    );
    openDropdown();

    fireEvent.click(screen.getByTestId(`${TAGS_FILTER_TEST_ID}-option-bug`));

    expect(onPluginFilterChange).toHaveBeenCalledWith(TAGS_FILTER_KEY, ["bug"]);
  });
});

describe("KanbanDisplayDropdown — plugin task filter identities", () => {
  it("keeps same-id filters from different plugins independent", () => {
    const onPluginFilterChange = vi.fn();
    const firstFilterKey = TAGS_FILTER_KEY;
    const secondFilterKey = PRIORITY_FILTER_KEY;
    const secondFilter = {
      pluginId: PRIORITY_PLUGIN_ID,
      id: TAGS_FILTER_ID,
      label: "Priority",
      getOptions: () => [{ value: "high", label: "High" }],
      matches: () => true,
    };

    render(
      <KanbanDisplayDropdown
        pluginFilters={[TAGS_FILTER, secondFilter]}
        pluginFilterSelections={{ [firstFilterKey]: ["bug"] }}
        onPluginFilterChange={onPluginFilterChange}
      />,
    );
    openDropdown();

    expect(
      screen
        .getByTestId(`display-plugin-filter-${firstFilterKey}-option-bug`)
        .getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe("checked");
    expect(
      screen
        .getByTestId(`display-plugin-filter-${secondFilterKey}-option-high`)
        .getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe("unchecked");

    fireEvent.click(screen.getByTestId(`display-plugin-filter-${secondFilterKey}-option-high`));

    expect(onPluginFilterChange).toHaveBeenCalledWith(secondFilterKey, ["high"]);
  });

  it("memoizes options while a filter registration remains stable", () => {
    const getOptions = vi.fn(() => [{ value: "bug", label: "Bug" }]);
    const filter = {
      pluginId: TAGS_PLUGIN_ID,
      id: TAGS_FILTER_ID,
      label: TAGS_FILTER.label,
      getOptions,
      matches: () => true,
    };
    const props = { pluginFilters: [filter] };
    const { rerender } = render(<KanbanDisplayDropdown {...props} />);
    openDropdown();

    expect(getOptions).toHaveBeenCalledTimes(1);

    rerender(<KanbanDisplayDropdown {...props} />);

    expect(getOptions).toHaveBeenCalledTimes(1);
  });
});

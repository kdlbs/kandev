import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { KanbanDisplayDropdown } from "./kanban-display-dropdown";

const { useKanbanDisplaySettingsMock } = vi.hoisted(() => ({
  useKanbanDisplaySettingsMock: vi.fn(),
}));

vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: useKanbanDisplaySettingsMock,
}));

function defaultMockSettings() {
  return {
    workflows: [],
    activeWorkflowId: null,
    repositories: [],
    repositoriesLoading: false,
    allRepositoriesSelected: true,
    selectedRepositoryId: null,
    enablePreviewOnClick: false,
    tasksListShowDetails: false,
    eligibleWorkflows: [],
    snapshots: {},
    hiddenWorkflowStepIds: {},
    onWorkflowChange: vi.fn(),
    onRepositoryChange: vi.fn(),
    onTogglePreviewOnClick: vi.fn(),
    onToggleTasksListShowDetails: vi.fn(),
    onToggleStepVisibility: vi.fn(),
  };
}

beforeEach(() => {
  useKanbanDisplaySettingsMock.mockReturnValue(defaultMockSettings());
});

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

const STEPS_WF_A_ID = "wf-a";
const STEPS_WF_B_ID = "wf-b";
const STEPS_WF_A = { id: STEPS_WF_A_ID, name: "Workflow A" };
const STEPS_WF_B = { id: STEPS_WF_B_ID, name: "Workflow B" };

describe("KanbanDisplayDropdown — Steps section", () => {
  it("lists a workflow's steps in position order, tiebreaking by id", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultMockSettings(),
      eligibleWorkflows: [STEPS_WF_A],
      snapshots: {
        [STEPS_WF_A_ID]: {
          steps: [
            { id: "step-z", title: "Z step", position: 1 },
            { id: "step-a", title: "A step", position: 0 },
            { id: "step-b-tie", title: "B tie", position: 1 },
          ],
        },
      },
    });
    render(<KanbanDisplayDropdown />);
    openDropdown();

    const group = screen.getByTestId(`steps-filter-group-${STEPS_WF_A_ID}`);
    const labels = Array.from(group.querySelectorAll("span.text-sm")).map((el) => el.textContent);
    // position 0 first; the two position-1 steps tiebreak ascending by id
    // ("step-b-tie" < "step-z" lexicographically).
    expect(labels).toEqual(["A step", "B tie", "Z step"]);
  });

  it("shows workflow name labels only when more than one workflow group has steps", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultMockSettings(),
      eligibleWorkflows: [STEPS_WF_A],
      snapshots: {
        [STEPS_WF_A_ID]: { steps: [{ id: "step-1", title: "Step 1", position: 0 }] },
      },
    });
    const { unmount } = render(<KanbanDisplayDropdown />);
    openDropdown();
    expect(screen.queryByText("Workflow A")).toBeNull();
    unmount();

    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultMockSettings(),
      eligibleWorkflows: [STEPS_WF_A, STEPS_WF_B],
      snapshots: {
        [STEPS_WF_A_ID]: { steps: [{ id: "step-1", title: "Step 1", position: 0 }] },
        [STEPS_WF_B_ID]: { steps: [{ id: "step-2", title: "Step 2", position: 0 }] },
      },
    });
    render(<KanbanDisplayDropdown />);
    openDropdown();
    expect(screen.getByText("Workflow A")).not.toBeNull();
    expect(screen.getByText("Workflow B")).not.toBeNull();
  });

  it("still renders a group for an eligible workflow whose snapshot has zero steps", () => {
    // Spec's Steps-section scope is "one group per workflow with a loaded
    // snapshot" — a workflow with literally zero steps is not carved out, so
    // it still gets a group (with no checkboxes inside) rather than being
    // silently dropped from the filter surface.
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultMockSettings(),
      eligibleWorkflows: [STEPS_WF_A, STEPS_WF_B],
      snapshots: {
        [STEPS_WF_A_ID]: { steps: [{ id: "step-1", title: "Step 1", position: 0 }] },
        [STEPS_WF_B_ID]: { steps: [] },
      },
    });
    render(<KanbanDisplayDropdown />);
    openDropdown();

    expect(screen.getByTestId(`steps-filter-group-${STEPS_WF_A_ID}`)).not.toBeNull();
    const groupB = screen.getByTestId(`steps-filter-group-${STEPS_WF_B_ID}`);
    expect(groupB).not.toBeNull();
    expect(groupB.querySelectorAll('[data-testid^="steps-filter-step-"]')).toHaveLength(0);
  });

  it("renders nothing when there are no eligible workflows at all", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultMockSettings(),
      eligibleWorkflows: [],
      snapshots: {},
    });
    render(<KanbanDisplayDropdown />);
    openDropdown();

    // StepsSection returns null entirely only when there is no eligible
    // workflow group to show at all — a workflow with zero steps still gets
    // an (empty) group, per the test above.
    expect(screen.queryByText("kanban:steps")).toBeNull();
    expect(screen.queryByTestId(/steps-filter-group-/)).toBeNull();
  });
});

describe("KanbanDisplayDropdown — Steps section checkbox interaction", () => {
  it("reflects checked/unchecked state from hiddenWorkflowStepIds", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultMockSettings(),
      eligibleWorkflows: [STEPS_WF_A],
      snapshots: {
        [STEPS_WF_A_ID]: {
          steps: [
            { id: "step-shown", title: "Shown", position: 0 },
            { id: "step-hidden", title: "Hidden", position: 1 },
          ],
        },
      },
      hiddenWorkflowStepIds: { [STEPS_WF_A_ID]: ["step-hidden"] },
    });
    render(<KanbanDisplayDropdown />);
    openDropdown();

    expect(screen.getByTestId("steps-filter-step-step-shown").getAttribute("data-state")).toBe(
      "checked",
    );
    expect(screen.getByTestId("steps-filter-step-step-hidden").getAttribute("data-state")).toBe(
      "unchecked",
    );
  });

  it("calls onToggleStepVisibility with the workflow and step id on click", () => {
    const onToggleStepVisibility = vi.fn();
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultMockSettings(),
      eligibleWorkflows: [STEPS_WF_A],
      snapshots: {
        [STEPS_WF_A_ID]: { steps: [{ id: "step-1", title: "Step 1", position: 0 }] },
      },
      onToggleStepVisibility,
    });
    render(<KanbanDisplayDropdown />);
    openDropdown();

    fireEvent.click(screen.getByTestId("steps-filter-step-step-1"));

    expect(onToggleStepVisibility).toHaveBeenCalledWith(STEPS_WF_A_ID, "step-1");
  });

  it("does not render the Steps section on the tasks page", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultMockSettings(),
      eligibleWorkflows: [STEPS_WF_A],
      snapshots: {
        [STEPS_WF_A_ID]: { steps: [{ id: "step-1", title: "Step 1", position: 0 }] },
      },
    });
    render(<KanbanDisplayDropdown currentPage="tasks" />);
    openDropdown();

    expect(screen.queryByTestId(`steps-filter-group-${STEPS_WF_A_ID}`)).toBeNull();
  });
});

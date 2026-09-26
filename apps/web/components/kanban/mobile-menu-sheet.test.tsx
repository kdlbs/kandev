import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps, HTMLAttributes, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MobileMenuSheet } from "./mobile-menu-sheet";

const { useKanbanDisplaySettingsMock, breakpointMocks, storeMocks, pluginFilterMocks } = vi.hoisted(
  () => ({
    useKanbanDisplaySettingsMock: vi.fn(),
    breakpointMocks: { isMobile: true, isTablet: false, breakpoint: "mobile" as string },
    storeMocks: { focusedWorkflowId: null as string | null },
    pluginFilterMocks: {
      filters: [] as Array<{
        pluginId: string;
        id: string;
        label: string;
        getOptions: () => Array<{ value: string; label: string }>;
        matches: () => boolean;
      }>,
      selections: {} as Record<string, string[]>,
      setFilterSelection: vi.fn(),
    },
  }),
);

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ mobileKanban: { focusedWorkflowId: storeMocks.focusedWorkflowId } }),
}));

afterEach(cleanup);

vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: useKanbanDisplaySettingsMock,
}));

vi.mock("@/hooks/use-plugin-task-filters", () => ({
  usePluginTaskFilters: () => pluginFilterMocks,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpointMocks,
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("@/components/app-sidebar/app-sidebar-workspace-picker", () => ({
  AppSidebarWorkspacePicker: () => null,
}));
// Plugins, integrations and the utility tail now come from the one shared nav
// block, which reaches the store and the health poller — out of scope here.
vi.mock("@/components/navigation/app-nav-sections", () => ({
  AppNavSections: () => null,
  useAppNavDialogs: () => ({
    showHealthRow: false,
    openImproveKandev: vi.fn(),
    openHealthDialog: vi.fn(),
    dialogs: null,
  }),
}));
vi.mock("./task-search-input", () => ({
  TaskSearchInput: () => null,
}));

vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children }: { children: ReactNode }) => <>{children}</>,
  DrawerContent: ({ children, ...props }: HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  DrawerHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DrawerTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));
vi.mock("@kandev/ui/sheet", () => ({
  Sheet: ({ children }: { children: ReactNode }) => <>{children}</>,
  SheetContent: ({ children, ...props }: HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  SheetHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SheetTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));

function defaultDisplaySettings() {
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
    workflowIdsWithAutoHideEmptySteps: [],
    boardSort: "created_desc" as "created_desc" | "priority_desc",
    priorityFilterTokens: [] as string[],
    onWorkflowChange: vi.fn(),
    onRepositoryChange: vi.fn(),
    onTogglePreviewOnClick: vi.fn(),
    onToggleTasksListShowDetails: vi.fn(),
    onToggleStepVisibility: vi.fn(),
    onToggleAutoHideEmpty: vi.fn(),
    effectiveTaskListingView: "kanban",
    onViewModeChange: vi.fn(),
    onBoardSortChange: vi.fn(),
    onPriorityFilterChange: vi.fn(),
  };
}

function renderSheet(props: Partial<ComponentProps<typeof MobileMenuSheet>> = {}) {
  return render(<MobileMenuSheet open onOpenChange={vi.fn()} currentPage="kanban" {...props} />);
}

function expandGroup(group: "filters" | "sort" | "preview" | "list-rows") {
  fireEvent.click(mobileGroupToggle(group));
}

const MOBILE_DISPLAY_SETTINGS_PREFIX = "mobile-display-settings-";
const MOBILE_BOARD_SORT_TEST_ID = "mobile-board-sort";
const MOBILE_CRITICAL_PRIORITY_TEST_ID = "mobile-priority-filter-option-critical";
const ARIA_EXPANDED_ATTRIBUTE = "aria-expanded";

function mobileGroupToggle(group: "filters" | "sort" | "preview" | "list-rows") {
  return screen.getByTestId(`${MOBILE_DISPLAY_SETTINGS_PREFIX}${group}-toggle`);
}

beforeEach(() => {
  useKanbanDisplaySettingsMock.mockReturnValue(defaultDisplaySettings());
  breakpointMocks.isMobile = true;
  breakpointMocks.isTablet = false;
  breakpointMocks.breakpoint = "mobile";
  storeMocks.focusedWorkflowId = null;
  pluginFilterMocks.filters = [];
  pluginFilterMocks.selections = {};
  pluginFilterMocks.setFilterSelection.mockClear();
});

describe("MobileMenuSheet — grouped display settings", () => {
  it("starts collapsed and expands each available group independently", () => {
    renderSheet();

    for (const group of ["filters", "sort", "preview"] as const) {
      expect(mobileGroupToggle(group).getAttribute(ARIA_EXPANDED_ATTRIBUTE)).toBe("false");
    }
    expect(screen.queryByTestId(MOBILE_BOARD_SORT_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(MOBILE_CRITICAL_PRIORITY_TEST_ID)).toBeNull();

    expandGroup("filters");
    expect(screen.getByTestId(MOBILE_CRITICAL_PRIORITY_TEST_ID)).not.toBeNull();
    expect(mobileGroupToggle("sort").getAttribute(ARIA_EXPANDED_ATTRIBUTE)).toBe("false");

    expandGroup("sort");
    expect(screen.getByTestId(MOBILE_BOARD_SORT_TEST_ID)).not.toBeNull();
    expandGroup("preview");
    expect(screen.getByTestId("mobile-display-preview-toggle")).not.toBeNull();
  });

  it("uses the same summaries and keeps list rows page-specific", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      workflows: [{ id: "wf-a", name: "Workflow A" }],
      activeWorkflowId: "wf-a",
      repositories: [{ id: "repo-a", name: "Repository A" }],
      allRepositoriesSelected: false,
      selectedRepositoryId: "repo-a",
      enablePreviewOnClick: true,
      boardSort: "priority_desc",
      priorityFilterTokens: ["critical"],
    });
    pluginFilterMocks.filters = [
      {
        pluginId: "plugin-a",
        id: "tags",
        label: "Tags",
        getOptions: () => [{ value: "bug", label: "Bug" }],
        matches: () => true,
      },
    ];
    pluginFilterMocks.selections = { "plugin-a:tags": ["bug"] };
    renderSheet();

    expect(screen.getByTestId("mobile-display-settings-filters").textContent).toContain(
      "Repository A, Critical, Tags: 1 selected",
    );
    expect(screen.getByTestId("mobile-display-settings-sort").textContent).toContain("Priority");
    expect(screen.getByTestId("mobile-display-settings-preview").textContent).toContain("On");

    cleanup();
    renderSheet({ currentPage: "tasks" });
    expect(mobileGroupToggle("list-rows").getAttribute(ARIA_EXPANDED_ATTRIBUTE)).toBe("false");
    expect(mobileGroupToggle("preview").getAttribute(ARIA_EXPANDED_ATTRIBUTE)).toBe("false");
    expect(screen.queryByTestId("mobile-display-preview-toggle")).toBeNull();
    expandGroup("list-rows");
    expect(screen.getByTestId("mobile-display-task-details-toggle")).not.toBeNull();
  });
});
describe("MobileMenuSheet — no Steps section (relocated to the lane)", () => {
  it("renders no step control on the phone kanban page", () => {
    // The dropdown-era Steps section is gone; the phone regains column
    // visibility as a focused-workflow block in task-03.
    renderSheet();

    expect(screen.queryByTestId(/steps-filter-/)).toBeNull();
    expect(screen.queryByTestId("steps-filter-section")).toBeNull();
  });
});

const WF_A = { id: "wf-a", name: "Workflow A" };
const WF_B = { id: "wf-b", name: "Workflow B" };
const TWO_WORKFLOWS = {
  eligibleWorkflows: [WF_A, WF_B],
  snapshots: {
    [WF_A.id]: { steps: [{ id: "a1", title: "Step A1", position: 0 }] },
    [WF_B.id]: { steps: [{ id: "b1", title: "Step B1", position: 0 }] },
  },
};

describe("MobileMenuSheet — Columns control for the focused workflow", () => {
  it("renders the Columns menu for the focused workflow only", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({ ...defaultDisplaySettings(), ...TWO_WORKFLOWS });
    storeMocks.focusedWorkflowId = WF_A.id;
    renderSheet();

    expect(screen.getByTestId(`columns-menu-${WF_A.id}`)).toBeTruthy();
    expect(screen.queryByTestId(`columns-menu-${WF_B.id}`)).toBeNull();
  });

  it("renders nothing when the board has reported no focused workflow", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({ ...defaultDisplaySettings(), ...TWO_WORKFLOWS });
    storeMocks.focusedWorkflowId = null;
    renderSheet();

    expect(screen.queryByTestId(/columns-menu-/)).toBeNull();
  });

  it("renders nothing off the phone, where the lane header owns the control", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({ ...defaultDisplaySettings(), ...TWO_WORKFLOWS });
    storeMocks.focusedWorkflowId = WF_A.id;
    breakpointMocks.isMobile = false;
    renderSheet();

    expect(screen.queryByTestId(/columns-menu-/)).toBeNull();
  });

  it("renders nothing on the tasks page", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({ ...defaultDisplaySettings(), ...TWO_WORKFLOWS });
    storeMocks.focusedWorkflowId = WF_A.id;
    renderSheet({ currentPage: "tasks" });

    expect(screen.queryByTestId(/columns-menu-/)).toBeNull();
  });

  it("hides controls that Threads does not apply", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      repositories: [{ id: "repo-1", name: "Repository 1" }],
    });

    renderSheet({ currentPage: "threads" });

    expect(screen.queryByText("Repository")).toBeNull();
    expect(screen.queryByText("Preview panel")).toBeNull();
    expandGroup("filters");
    expect(screen.getByTestId("mobile-display-workflow-filter")).not.toBeNull();
  });
});

describe("MobileMenuSheet — board sort and priority filter", () => {
  it("renders the board sort select and priority filter checkboxes on the phone kanban page", () => {
    renderSheet({ currentPage: "kanban" });
    expandGroup("filters");
    expandGroup("sort");

    expect(screen.getByTestId(MOBILE_BOARD_SORT_TEST_ID)).not.toBeNull();
    expect(screen.getByTestId(MOBILE_CRITICAL_PRIORITY_TEST_ID)).not.toBeNull();
    expect(screen.getByTestId("mobile-priority-filter-option-high")).not.toBeNull();
    expect(screen.getByTestId("mobile-priority-filter-option-medium")).not.toBeNull();
    expect(screen.getByTestId("mobile-priority-filter-option-low")).not.toBeNull();
  });

  it("names the board sort select by what it selects, not its current value", () => {
    renderSheet({ currentPage: "kanban" });
    expandGroup("sort");

    expect(screen.getByRole("combobox", { name: "Board sort" })).not.toBeNull();
  });

  it("renders nothing on the tasks page", () => {
    renderSheet({ currentPage: "tasks" });

    expect(screen.queryByTestId(MOBILE_BOARD_SORT_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(/mobile-priority-filter-option-/)).toBeNull();
  });

  it("reflects the current priority filter selection", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      priorityFilterTokens: ["critical"],
    });
    renderSheet({ currentPage: "kanban" });
    expandGroup("filters");

    expect(screen.getByTestId(MOBILE_CRITICAL_PRIORITY_TEST_ID).getAttribute("data-state")).toBe(
      "checked",
    );
    expect(
      screen.getByTestId("mobile-priority-filter-option-high").getAttribute("data-state"),
    ).toBe("unchecked");
  });

  it("invokes onPriorityFilterChange when tapping a priority option", () => {
    const onPriorityFilterChange = vi.fn();
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      onPriorityFilterChange,
    });
    renderSheet({ currentPage: "kanban" });
    expandGroup("filters");

    screen.getByTestId("mobile-priority-filter-option-high").click();

    expect(onPriorityFilterChange).toHaveBeenCalledWith("high");
  });
});

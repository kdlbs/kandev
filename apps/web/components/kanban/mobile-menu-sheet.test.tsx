import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps, HTMLAttributes, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MobileMenuSheet } from "./mobile-menu-sheet";

const { useKanbanDisplaySettingsMock, breakpointMocks } = vi.hoisted(() => ({
  useKanbanDisplaySettingsMock: vi.fn(),
  breakpointMocks: { isMobile: true, isTablet: false, breakpoint: "mobile" as string },
}));

vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: useKanbanDisplaySettingsMock,
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
vi.mock("@/components/integrations/integrations-menu", () => ({
  MobileIntegrationsSection: () => null,
}));
vi.mock("@/components/plugins/mobile-plugin-nav-section", () => ({
  MobilePluginNavSection: () => null,
}));
vi.mock("./mobile-menu-utility-actions", () => ({
  MobileUtilityActions: () => null,
}));
vi.mock("@/components/improve-kandev-dialog", () => ({
  ImproveKandevDialog: () => null,
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

const WF_A = { id: "wf-a", name: "Workflow A" };
const WF_B = { id: "wf-b", name: "Workflow B" };

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
    onWorkflowChange: vi.fn(),
    onRepositoryChange: vi.fn(),
    onTogglePreviewOnClick: vi.fn(),
    onToggleTasksListShowDetails: vi.fn(),
    onToggleStepVisibility: vi.fn(),
    effectiveTaskListingView: "kanban",
    onViewModeChange: vi.fn(),
  };
}

function renderSheet(props: Partial<ComponentProps<typeof MobileMenuSheet>> = {}) {
  return render(
    <MobileMenuSheet
      open
      onOpenChange={vi.fn()}
      currentPage="kanban"
      showHealthIndicator={false}
      onOpenHealthDialog={vi.fn()}
      {...props}
    />,
  );
}

const TWO_WORKFLOW_SNAPSHOTS = {
  [WF_A.id]: { steps: [{ id: "a1", title: "Step A1", position: 0 }] },
  [WF_B.id]: { steps: [{ id: "b1", title: "Step B1", position: 0 }] },
} as never;

beforeEach(() => {
  useKanbanDisplaySettingsMock.mockReturnValue(defaultDisplaySettings());
  breakpointMocks.isMobile = true;
  breakpointMocks.isTablet = false;
  breakpointMocks.breakpoint = "mobile";
});

afterEach(cleanup);

describe("MobileMenuSheet — Steps section surface exclusivity (item 2, F25)", () => {
  it("renders the Steps section on a phone kanban page", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      eligibleWorkflows: [WF_A],
      snapshots: { [WF_A.id]: { steps: [{ id: "s1", title: "Step 1", position: 0 }] } },
    });
    renderSheet();
    expect(screen.getByTestId(`steps-filter-group-${WF_A.id}`)).not.toBeNull();
  });

  it("does not render the Steps section on a tablet viewport — the drawer subtree holds zero steps-filter-* testids", () => {
    breakpointMocks.isMobile = false;
    breakpointMocks.isTablet = true;
    breakpointMocks.breakpoint = "tablet";
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      eligibleWorkflows: [WF_A],
      snapshots: { [WF_A.id]: { steps: [{ id: "s1", title: "Step 1", position: 0 }] } },
    });
    renderSheet();
    expect(screen.queryByTestId(/^steps-filter-/)).toBeNull();
  });

  it("does not render the Steps section on the tasks page on a phone viewport (F25)", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      eligibleWorkflows: [WF_A],
      snapshots: { [WF_A.id]: { steps: [{ id: "s1", title: "Step 1", position: 0 }] } },
    });
    renderSheet({ currentPage: "tasks" });
    expect(screen.queryByTestId(/^steps-filter-/)).toBeNull();
  });
});

describe("MobileMenuSheet — Steps section wiring", () => {
  it("toggling a step checkbox invokes onToggleStepVisibility with workflow and step id", () => {
    const onToggleStepVisibility = vi.fn();
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      eligibleWorkflows: [WF_A],
      snapshots: { [WF_A.id]: { steps: [{ id: "s1", title: "Step 1", position: 0 }] } },
      onToggleStepVisibility,
    });
    renderSheet();
    fireEvent.click(screen.getByTestId("steps-filter-step-s1"));
    expect(onToggleStepVisibility).toHaveBeenCalledWith(WF_A.id, "s1");
  });
});

describe("MobileMenuSheet — override-reset rule for the held-open breakpoint crossing (item 6)", () => {
  it("clears a disclosure override after mobile -> tablet -> mobile with the drawer held open throughout", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      eligibleWorkflows: [WF_A, WF_B],
      snapshots: TWO_WORKFLOW_SNAPSHOTS,
    });
    const { rerender } = renderSheet();
    const rerenderSheet = () =>
      rerender(
        <MobileMenuSheet
          open
          onOpenChange={vi.fn()}
          currentPage="kanban"
          showHealthIndicator={false}
          onOpenHealthDialog={vi.fn()}
        />,
      );

    // Explicitly expand A's group (collapsed by default; nothing hidden).
    fireEvent.click(screen.getByTestId(`steps-filter-group-toggle-${WF_A.id}`));
    expect(
      screen.getByTestId(`steps-filter-group-toggle-${WF_A.id}`).getAttribute("aria-expanded"),
    ).toBe("true");

    // Widen into the tablet range — the drawer's `open` prop stays true throughout.
    breakpointMocks.isMobile = false;
    breakpointMocks.isTablet = true;
    breakpointMocks.breakpoint = "tablet";
    rerenderSheet();

    // Narrow back below 768px, still held open.
    breakpointMocks.isMobile = true;
    breakpointMocks.isTablet = false;
    breakpointMocks.breakpoint = "mobile";
    rerenderSheet();

    expect(
      screen.getByTestId(`steps-filter-group-toggle-${WF_A.id}`).getAttribute("aria-expanded"),
    ).toBe("false");
    expect(screen.queryByTestId("steps-filter-step-a1")).toBeNull();
  });

  it("does NOT reset the override while the drawer stays on the mobile surface with no crossing", () => {
    useKanbanDisplaySettingsMock.mockReturnValue({
      ...defaultDisplaySettings(),
      eligibleWorkflows: [WF_A, WF_B],
      snapshots: TWO_WORKFLOW_SNAPSHOTS,
    });
    const { rerender } = renderSheet();
    fireEvent.click(screen.getByTestId(`steps-filter-group-toggle-${WF_A.id}`));

    rerender(
      <MobileMenuSheet
        open
        onOpenChange={vi.fn()}
        currentPage="kanban"
        showHealthIndicator={false}
        onOpenHealthDialog={vi.fn()}
      />,
    );

    expect(
      screen.getByTestId(`steps-filter-group-toggle-${WF_A.id}`).getAttribute("aria-expanded"),
    ).toBe("true");
  });
});

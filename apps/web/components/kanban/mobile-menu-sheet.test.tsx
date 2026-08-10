import { cleanup, render, screen } from "@testing-library/react";
import type { ComponentProps, HTMLAttributes, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MobileMenuSheet } from "./mobile-menu-sheet";

const { useKanbanDisplaySettingsMock, breakpointMocks, storeMocks } = vi.hoisted(() => ({
  useKanbanDisplaySettingsMock: vi.fn(),
  breakpointMocks: { isMobile: true, isTablet: false, breakpoint: "mobile" as string },
  storeMocks: { focusedWorkflowId: null as string | null },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ mobileKanban: { focusedWorkflowId: storeMocks.focusedWorkflowId } }),
}));

afterEach(cleanup);

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

beforeEach(() => {
  useKanbanDisplaySettingsMock.mockReturnValue(defaultDisplaySettings());
  breakpointMocks.isMobile = true;
  breakpointMocks.isTablet = false;
  breakpointMocks.breakpoint = "mobile";
  storeMocks.focusedWorkflowId = null;
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
});

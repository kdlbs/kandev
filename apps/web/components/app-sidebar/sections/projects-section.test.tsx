import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  mode: "kanban" as "kanban" | "office" | "unknown",
  isMobile: false,
  projectsEnabled: true,
  activeProjects: [] as Array<Record<string, unknown>>,
  archivedProjects: [] as Array<Record<string, unknown>>,
  officeProjects: [] as Array<Record<string, unknown>>,
}));

const state = {
  workspaces: { activeId: "ws-1" },
  office: { projectsByWorkspaceId: { "ws-1": mocks.officeProjects } },
  appSidebar: { sectionExpanded: {} },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
};

vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => mocks }));
vi.mock("@/hooks/use-in-office", () => ({ useOfficeModeState: () => mocks.mode }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile }),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mocks.projectsEnabled,
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/hooks/domains/agent-projects/use-agent-projects", () => ({
  useAgentProjects: (_workspaceId: string, archived: boolean) => ({
    projects: archived ? mocks.archivedProjects : mocks.activeProjects,
    loaded: true,
    loading: false,
    error: null,
    refresh: vi.fn(),
  }),
  useAgentProjectMutations: () => ({ restore: vi.fn() }),
  projectWorkers: (project: {
    tasks: Array<{ id: string; parent_id?: string }>;
    main_task_id: string;
  }) => project.tasks.filter((task) => task.parent_id === project.main_task_id),
}));
vi.mock("@/components/agent-projects/agent-project-form-dialog", () => ({
  AgentProjectFormDialog: () => null,
}));
vi.mock("@/components/agent-projects/agent-project-action-dialog", () => ({
  AgentProjectActionDialog: () => null,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (snapshot: typeof state) => unknown) => selector(state),
}));
vi.mock("@/components/app-sidebar/app-sidebar-section", () => ({
  AppSidebarSection: ({
    label,
    headerAction,
    children,
  }: React.PropsWithChildren<{ label: string; headerAction?: React.ReactNode }>) => (
    <section>
      <h2>{label}</h2>
      {headerAction}
      {children}
    </section>
  ),
}));
vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { ProjectsSection } from "./projects-section";

const project = (
  id: string,
  tasks: Array<Record<string, unknown>> = [
    { id: "main-" + id, title: id, state: "IN_PROGRESS", updated_at: "" },
  ],
) =>
  ({
    id,
    name: id,
    main_task_id: "main-" + id,
    tasks,
    repository_ids: [],
    revision: 1,
  }) as never;

describe("ProjectsSection", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    mocks.mode = "kanban";
    mocks.isMobile = false;
    mocks.projectsEnabled = true;
    mocks.activeProjects = [];
    mocks.archivedProjects = [];
    mocks.officeProjects = [];
  });

  it("opens a coordinator and expands only direct worker rows", () => {
    mocks.activeProjects = [
      project("release", [
        { id: "main-release", title: "Release", state: "IN_PROGRESS", updated_at: "" },
        {
          id: "worker-1",
          title: "Update adapters",
          parent_id: "main-release",
          state: "COMPLETED",
          updated_at: "",
        },
      ]),
    ];

    render(<ProjectsSection collapsed={false} />);
    expect(screen.getByTestId("agent-project-expand-release").getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(screen.queryByTestId("agent-project-worker-worker-1")).toBeNull();
    fireEvent.click(screen.getByTestId("agent-project-open-release"));
    expect(mocks.push).toHaveBeenCalledWith("/t/main-release");
    fireEvent.click(screen.getByTestId("agent-project-expand-release"));
    expect(screen.getByTestId("agent-project-worker-worker-1")).toBeTruthy();
    fireEvent.click(screen.getByTestId("agent-project-worker-worker-1"));
    expect(mocks.push).toHaveBeenCalledWith("/t/worker-1");
  });

  it("does not render a worker disclosure for a project with no workers", () => {
    mocks.activeProjects = [project("empty")];
    render(<ProjectsSection collapsed={false} />);

    expect(screen.queryByTestId("agent-project-expand-empty")).toBeNull();
    expect(screen.getByTestId("agent-project-open-empty")).toBeTruthy();
  });

  it("keeps Office projects in the same section without mixing Agent Projects", () => {
    mocks.mode = "office";
    mocks.activeProjects = [project("agent-project")];
    mocks.officeProjects = [
      { id: "office-project", name: "Office Project", status: "active", taskCounts: { total: 2 } },
    ];
    state.office.projectsByWorkspaceId["ws-1"] = mocks.officeProjects as never;

    render(<ProjectsSection collapsed={false} />);

    expect(screen.getByText("Office Project")).toBeTruthy();
    expect(screen.queryByTestId("agent-project-open-agent-project")).toBeNull();
    fireEvent.click(screen.getByText("Office Project"));
    expect(mocks.push).toHaveBeenCalledWith("/office/projects/office-project");
  });

  it("hides the Agent Projects section when its release flag is off", () => {
    mocks.projectsEnabled = false;
    render(<ProjectsSection collapsed={false} />);
    expect(screen.queryByText("Projects")).toBeNull();
  });

  it("keeps the mobile project create target at least 44px square", () => {
    mocks.isMobile = true;
    render(<ProjectsSection collapsed={false} />);

    const button = screen.getByTestId("agent-project-create-open");
    expect(button.className).toContain("h-11");
    expect(button.className).toContain("w-11");
  });
});

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useSidebarShortcutCatalog } from "@/hooks/domains/sidebar/use-sidebar-shortcut-catalog";
import { LIVE_REFRESH_INTERVAL_MS } from "@/components/runs/use-live-refresh";
import { defaultSettingsState } from "@/lib/state/slices/settings/settings-slice";
import type { Automation } from "@/lib/types/automation";
import type { SidebarLayoutApi } from "@/lib/types/http-user-settings";
import { SidebarLayoutNavigation } from "./sidebar-layout-navigation";

const transport = vi.hoisted(() => ({ list: vi.fn(), summaries: vi.fn(), settings: vi.fn() }));
vi.mock("@/lib/api/domains/automation-api", async (original) => ({
  ...(await original<typeof import("@/lib/api/domains/automation-api")>()),
  listAutomations: transport.list,
  listAutomationSummaries: transport.summaries,
}));
vi.mock("@/lib/api/domains/settings-api", async (original) => ({
  ...(await original<typeof import("@/lib/api/domains/settings-api")>()),
  updateUserSettings: transport.settings,
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => false }));
vi.mock("@/hooks/use-in-office", () => ({ useOfficeModeState: () => "kanban" }));
vi.mock("@/hooks/use-app-destinations", () => ({
  useStaticDestinations: () => [],
  useAppDestinations: () => [],
}));
vi.mock("@/hooks/use-quick-chat-launcher", () => ({ useQuickChatLauncher: () => vi.fn() }));
vi.mock("@/hooks/use-quick-terminal-launcher", () => ({ useQuickTerminalLauncher: () => vi.fn() }));
vi.mock("./app-sidebar-new-task-item", () => ({ AppSidebarNewTaskItem: () => null }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

const A_ROW = "sidebar-automation-a-row";

let store: ReturnType<typeof useAppStoreApi>;
function CatalogProbe() {
  store = useAppStoreApi();
  const { automations } = useSidebarShortcutCatalog();
  return <output data-testid="catalog">{automations.map((row) => row.name).join(",")}</output>;
}
function row(workspace: string, name: string): Automation {
  return {
    id: `${workspace}-row`,
    workspace_id: workspace,
    name,
    description: "",
    workflow_id: "",
    workflow_step_id: "",
    agent_profile_id: "",
    executor_profile_id: "",
    repository_ids: [],
    prompt: "",
    task_title_template: "",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-10-23T00:00:00Z",
    updated_at: "2026-10-23T00:00:00Z",
    triggers: [],
  };
}
function layout(visible: boolean): SidebarLayoutApi {
  return {
    version: 1,
    revision: 7,
    nodes: [
      { id: "automations", kind: "builtin", destination_id: "automations", visible },
      { id: "other", kind: "shortcuts", name: "Other", visible: false, shortcuts: [] },
    ],
  };
}
function openSection() {
  const header = screen.getByRole("button", { name: /^Automations/ });
  if (header.getAttribute("aria-expanded") !== "true") fireEvent.click(header);
}
function mount(visible = true) {
  const view = render(
    <StateProvider
      initialState={{
        workspaces: { activeId: "a", items: [] },
        userSettings: {
          ...defaultSettingsState.userSettings,
          sidebarLayoutsByWorkspace: { a: layout(visible), b: layout(visible) },
        },
      }}
    >
      <TooltipProvider>
        <CatalogProbe />
        <SidebarLayoutNavigation collapsed={false} inOffice={false} />
      </TooltipProvider>
    </StateProvider>,
  );
  if (visible) openSection();
  return view;
}
beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  transport.list.mockReset().mockResolvedValue([row("a", "Before")]);
  transport.summaries.mockReset().mockResolvedValue([]);
  transport.settings.mockReset();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

// @covers AC-OFFICE-AUTOMATION-RUNS-001.10 AC-OFFICE-AUTOMATION-RUNS-001.12
it("refreshes catalog and builtin independently without changing saved navigation", async () => {
  mount();
  await act(async () => {});
  const section = document.getElementById("sidebar-section-automations");
  expect(screen.getByTestId(A_ROW).textContent).toContain("Before");
  const settings = store.getState().userSettings.sidebarLayoutsByWorkspace;
  const expansion = store.getState().appSidebar.sectionExpanded;
  transport.list.mockResolvedValue([row("a", "Renamed externally")]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
  });
  expect(screen.getByTestId("catalog").textContent).toBe("Renamed externally");
  expect(screen.getByTestId(A_ROW).textContent).toContain("Renamed externally");
  expect(document.getElementById("sidebar-section-automations")).toBe(section);
  expect(store.getState().userSettings.sidebarLayoutsByWorkspace).toEqual(settings);
  expect(store.getState().appSidebar.sectionExpanded).toEqual(expansion);
  expect(transport.settings).not.toHaveBeenCalled();
});

// @covers AC-OFFICE-AUTOMATION-RUNS-001.12
it("keeps a saved hidden builtin hidden when the catalog gains a definition", async () => {
  transport.list.mockResolvedValue([]);
  mount(false);
  await act(async () => {});
  const settings = store.getState().userSettings.sidebarLayoutsByWorkspace;
  transport.list.mockResolvedValue([row("a", "External creation")]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
  });
  expect(screen.getByTestId("catalog").textContent).toBe("External creation");
  expect(document.getElementById("sidebar-section-automations")).toBeNull();
  expect(store.getState().userSettings.sidebarLayoutsByWorkspace).toEqual(settings);
  expect(transport.settings).not.toHaveBeenCalled();
  // Positive control: the same real projection can render the builtin when visible.
  await act(async () =>
    store.setState((state) => ({
      userSettings: {
        ...state.userSettings,
        sidebarLayoutsByWorkspace: { ...settings, a: layout(true) },
      },
    })),
  );
  openSection();
  expect(screen.getByTestId(A_ROW).textContent).toContain("External creation");
});

// @covers AC-OFFICE-AUTOMATION-RUNS-001.11
it("keeps the real navigation in A after late B responses", async () => {
  mount();
  await act(async () => {});
  let release!: (rows: Automation[]) => void;
  const pending = new Promise<Automation[]>((resolve) => {
    release = resolve;
  });
  transport.list.mockImplementation((workspace: string) =>
    workspace === "b" ? pending : Promise.resolve([row("a", "Current A")]),
  );
  try {
    await act(async () => store.getState().setActiveWorkspace("b"));
    expect(screen.queryByTestId(A_ROW)).toBeNull();
    await act(async () => store.getState().setActiveWorkspace("a"));
    await act(async () => release([row("b", "Foreign B")]));
    expect(screen.queryByTestId("sidebar-automation-b-row")).toBeNull();
    expect(screen.getByTestId(A_ROW).textContent).toContain("Current A");
    expect(screen.getByTestId("catalog").textContent).toBe("Current A");
  } finally {
    await act(async () => release([]));
  }
});

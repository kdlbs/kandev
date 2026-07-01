import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  routerPush: vi.fn(),
  toggleSettingsMode: vi.fn(),
}));

const state = {
  workspaces: {
    activeId: "kanban-1" as string | null,
  },
  appSidebar: { settingsMode: false },
};

let officeEnabled = false;
let pathname = "/tasks/session-1";
let workspaceItems = [
  { id: "kanban-1", name: "Kanban", office_workflow_id: "" },
  { id: "office-1", name: "Office", office_workflow_id: "wf-office" },
  { id: "office-2", name: "Office 2", office_workflow_id: "wf-office-2" },
];

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.routerPush }),
  usePathname: () => pathname,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => officeEnabled,
}));

vi.mock("@/hooks/domains/workspace/use-workspaces", () => ({
  useWorkspaces: () => ({
    items: workspaceItems,
    activeId: state.workspaces.activeId,
    activeWorkspace: workspaceItems.find((workspace) => workspace.id === state.workspaces.activeId),
  }),
}));

vi.mock("@/hooks/use-release-notes", () => ({
  useReleaseNotes: () => ({
    unseenEntries: [],
    latestVersion: "0.0.0",
    hasUnseen: false,
    dialogOpen: false,
    openDialog: vi.fn(),
    closeDialog: vi.fn(),
    hasNotes: false,
    showTopbarButton: false,
  }),
}));

vi.mock("@/components/improve-kandev-dialog", () => ({
  ImproveKandevDialog: () => null,
}));

vi.mock("@/components/release-notes/release-notes-dialog", () => ({
  ReleaseNotesDialog: () => null,
}));

vi.mock("@/components/theme-toggle", () => ({
  ThemeToggle: () => <button type="button">Theme</button>,
}));

import { AppSidebarFooter } from "./app-sidebar-footer";

function renderFooter() {
  return render(
    <TooltipProvider>
      <AppSidebarFooter collapsed={false} onToggleSettingsMode={mocks.toggleSettingsMode} />
    </TooltipProvider>,
  );
}

describe("AppSidebarFooter", () => {
  beforeEach(() => {
    officeEnabled = false;
    pathname = "/tasks/session-1";
    state.workspaces.activeId = "kanban-1";
    workspaceItems = [
      { id: "kanban-1", name: "Kanban", office_workflow_id: "" },
      { id: "office-1", name: "Office", office_workflow_id: "wf-office" },
      { id: "office-2", name: "Office 2", office_workflow_id: "wf-office-2" },
    ];
    state.appSidebar.settingsMode = false;
    window.localStorage.clear();
    document.cookie = "office-active-workspace=; path=/; max-age=0";
    mocks.routerPush.mockClear();
    mocks.toggleSettingsMode.mockClear();
  });

  afterEach(() => cleanup());

  it("renders navigation icons as buttons so hover does not expose link URLs", () => {
    officeEnabled = true;

    renderFooter();

    const statsButton = screen.getByRole("button", { name: "Stats" });
    const officeButton = screen.getByRole("button", { name: "Office" });

    expect(statsButton).toBeTruthy();
    expect(officeButton).toBeTruthy();
    expect(statsButton.getAttribute("href")).toBeNull();
    expect(officeButton.getAttribute("href")).toBeNull();
    expect(screen.queryByRole("link", { name: "Stats" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Office" })).toBeNull();
  });

  it("navigates from the Stats and Office footer buttons when kanban is active", () => {
    officeEnabled = true;

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Stats" }));
    fireEvent.click(screen.getByRole("button", { name: "Office" }));

    expect(mocks.routerPush).toHaveBeenNthCalledWith(1, "/stats");
    expect(mocks.routerPush).toHaveBeenNthCalledWith(2, "/office?workspaceId=office-1");
    expect(window.localStorage.getItem("kandev.lastKanbanWorkspaceId")).toBe("kanban-1");
  });

  it("navigates to the last active office workspace when kanban is active", () => {
    officeEnabled = true;
    document.cookie = "office-active-workspace=office-2; path=/";

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Office" }));

    expect(mocks.routerPush).toHaveBeenCalledWith("/office?workspaceId=office-2");
  });

  it("navigates to office setup when no office workspace exists", () => {
    officeEnabled = true;
    workspaceItems = [{ id: "kanban-1", name: "Kanban", office_workflow_id: "" }];

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Office" }));

    expect(mocks.routerPush).toHaveBeenCalledWith("/office/setup?mode=new");
  });

  it("shows a Kanban button when an office workspace is active", () => {
    officeEnabled = true;
    state.workspaces.activeId = "office-1";
    window.localStorage.setItem("kandev.lastKanbanWorkspaceId", "kanban-1");

    renderFooter();

    expect(screen.queryByRole("button", { name: "Office" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Kanban" }));

    expect(mocks.routerPush).toHaveBeenCalledWith("/?workspaceId=kanban-1");
  });

  it("remembers the current office workspace when toggling back to kanban", () => {
    officeEnabled = true;
    state.workspaces.activeId = "office-2";
    window.localStorage.setItem("kandev.lastKanbanWorkspaceId", "kanban-1");

    renderFooter();

    fireEvent.click(screen.getByRole("button", { name: "Kanban" }));

    expect(document.cookie).toContain("office-active-workspace=office-2");
    expect(mocks.routerPush).toHaveBeenCalledWith("/?workspaceId=kanban-1");
  });
  it("navigates to /settings when the gear opens settings mode from a non-settings route", () => {
    pathname = "/tasks/session-1";
    state.appSidebar.settingsMode = false;

    renderFooter();
    fireEvent.click(screen.getByTestId("sidebar-settings-gear"));

    expect(mocks.routerPush).toHaveBeenCalledWith("/settings");
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });

  it("does not navigate when the gear reopens settings mode while already on a settings route", () => {
    pathname = "/settings/agents";
    state.appSidebar.settingsMode = false;

    renderFooter();
    fireEvent.click(screen.getByTestId("sidebar-settings-gear"));

    expect(mocks.routerPush).not.toHaveBeenCalled();
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });

  it("does not navigate when the gear closes an already-open settings mode", () => {
    pathname = "/settings";
    state.appSidebar.settingsMode = true;

    renderFooter();
    fireEvent.click(screen.getByTestId("sidebar-settings-gear"));

    expect(mocks.routerPush).not.toHaveBeenCalled();
    expect(mocks.toggleSettingsMode).toHaveBeenCalledOnce();
  });
});

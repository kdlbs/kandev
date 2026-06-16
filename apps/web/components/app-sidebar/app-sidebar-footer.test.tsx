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
    items: [
      { id: "kanban-1", name: "Kanban", office_workflow_id: "" },
      { id: "office-1", name: "Office", office_workflow_id: "wf-office" },
      { id: "office-2", name: "Office 2", office_workflow_id: "wf-office-2" },
    ],
  },
  appSidebar: { settingsMode: false },
  toggleAppSidebarSettingsMode: mocks.toggleSettingsMode,
};

let officeEnabled = false;

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.routerPush }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => officeEnabled,
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
      <AppSidebarFooter collapsed={false} />
    </TooltipProvider>,
  );
}

describe("AppSidebarFooter", () => {
  beforeEach(() => {
    officeEnabled = false;
    state.workspaces.activeId = "kanban-1";
    state.workspaces.items = [
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
    state.workspaces.items = [{ id: "kanban-1", name: "Kanban", office_workflow_id: "" }];

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
});

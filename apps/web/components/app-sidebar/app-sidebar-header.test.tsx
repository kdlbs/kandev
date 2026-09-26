import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkspaceScopeProvider } from "@/components/workspace-scope-provider";

const isMacTauriWebview = vi.hoisted(() => vi.fn(() => false));
vi.mock("@/lib/desktop/window-chrome", () => ({
  isMacTauriWebview,
  macTauriDragRegionProps: () => (isMacTauriWebview() ? { "data-tauri-drag-region": "deep" } : {}),
}));

const setWorkspacePickerOpen = vi.fn();

const state = {
  workspaces: {
    activeId: "kanban-1" as string | null,
    items: [
      { id: "kanban-1", name: "Kanban", office_workflow_id: "" },
      { id: "office-1", name: "Office", office_workflow_id: "wf-office" },
    ],
  },
  features: { office: true },
  userSettings: { startupPage: "task_overview" },
  appSidebar: { workspacePickerOpen: false },
  setWorkspacePickerOpen,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("./app-sidebar-workspace-picker", () => ({
  AppSidebarWorkspacePicker: ({
    open,
    onOpenChange,
  }: {
    open?: boolean;
    onOpenChange?: (open: boolean) => void;
  }) => (
    <button
      type="button"
      data-testid="workspace-picker"
      data-open={String(open)}
      onClick={() => onOpenChange?.(false)}
    />
  ),
}));

import { AppSidebarHeader } from "./app-sidebar-header";

function renderHeader(collapsed = false) {
  return render(
    <TooltipProvider>
      <WorkspaceScopeProvider>
        <AppSidebarHeader collapsed={collapsed} onToggleCollapse={vi.fn()} />
      </WorkspaceScopeProvider>
    </TooltipProvider>,
  );
}

describe("AppSidebarHeader", () => {
  beforeEach(() => {
    isMacTauriWebview.mockReturnValue(false);
    state.workspaces.activeId = "kanban-1";
    state.appSidebar.workspacePickerOpen = false;
    setWorkspacePickerOpen.mockClear();
  });

  afterEach(() => cleanup());

  it("routes the Kandev brand to the active kanban workspace home", () => {
    renderHeader();

    expect(screen.getByRole("link", { name: "Kandev home" }).getAttribute("href")).toBe(
      "/?home=overview&workspaceId=kanban-1",
    );
  });

  it("routes the Kandev brand to the active office workspace home", () => {
    state.workspaces.activeId = "office-1";

    renderHeader();

    expect(screen.getByRole("link", { name: "Kandev home" }).getAttribute("href")).toBe(
      "/office?workspaceId=office-1",
    );
  });

  it("drives the workspace picker from the store flag", () => {
    state.appSidebar.workspacePickerOpen = true;

    renderHeader();

    expect(screen.getByTestId("workspace-picker").getAttribute("data-open")).toBe("true");
  });

  it("writes the picker's dismissal back to the store", () => {
    state.appSidebar.workspacePickerOpen = true;

    renderHeader();
    screen.getByTestId("workspace-picker").click();

    expect(setWorkspacePickerOpen).toHaveBeenCalledWith(false);
  });

  it("marks the expanded header as a native drag region on macOS Tauri", () => {
    isMacTauriWebview.mockReturnValue(true);

    renderHeader();

    const header = screen.getByTestId("app-sidebar-header");
    expect(header.getAttribute("data-tauri-drag-region")).toBe("deep");
    expect(header.getAttribute("data-sidebar-header-collapsed")).toBe("false");
  });

  it("keeps the collapsed brand and expand control below the native controls", () => {
    isMacTauriWebview.mockReturnValue(true);

    renderHeader(true);

    const header = screen.getByTestId("app-sidebar-header");
    expect(header.getAttribute("data-sidebar-header-collapsed")).toBe("true");
    expect(header.getAttribute("data-tauri-drag-region")).toBe("deep");
  });
});

it("offers permanent expansion while displaying the hover-revealed workspace picker", () => {
  const toggle = vi.fn();
  render(
    <TooltipProvider>
      <WorkspaceScopeProvider>
        <AppSidebarHeader collapsed={false} hoverRevealed onToggleCollapse={toggle} />
      </WorkspaceScopeProvider>
    </TooltipProvider>,
  );
  expect(screen.getByTestId("workspace-picker")).toBeTruthy();
  screen.getByRole("button", { name: "Expand sidebar" }).click();
  expect(toggle).toHaveBeenCalledOnce();
});

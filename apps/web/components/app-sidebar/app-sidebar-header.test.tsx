import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkspaceScopeProvider } from "@/components/workspace-scope-provider";

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
});

import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = {
  workspaces: {
    activeId: "kanban-1" as string | null,
    items: [
      { id: "kanban-1", name: "Kanban", office_workflow_id: "" },
      { id: "office-1", name: "Office", office_workflow_id: "wf-office" },
    ],
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("./app-sidebar-workspace-picker", () => ({
  AppSidebarWorkspacePicker: () => <div data-testid="workspace-picker" />,
}));

import { AppSidebarHeader } from "./app-sidebar-header";

function renderHeader(collapsed = false) {
  return render(
    <TooltipProvider>
      <AppSidebarHeader collapsed={collapsed} onToggleCollapse={vi.fn()} />
    </TooltipProvider>,
  );
}

describe("AppSidebarHeader", () => {
  beforeEach(() => {
    state.workspaces.activeId = "kanban-1";
  });

  afterEach(() => cleanup());

  it("routes the Kandev brand to the active kanban workspace home", () => {
    renderHeader();

    expect(screen.getByRole("link", { name: "Kandev home" }).getAttribute("href")).toBe(
      "/?workspaceId=kanban-1",
    );
  });

  it("routes the Kandev brand to the active office workspace home", () => {
    state.workspaces.activeId = "office-1";

    renderHeader();

    expect(screen.getByRole("link", { name: "Kandev home" }).getAttribute("href")).toBe(
      "/office?workspaceId=office-1",
    );
  });
});

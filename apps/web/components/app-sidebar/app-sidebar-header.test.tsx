import { cleanup, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";

const state = {
  workspaces: {
    activeId: "kanban-1" as string | null,
  },
  setActiveWorkspace: vi.fn(),
  setActiveWorkflow: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("./app-sidebar-workspace-picker", () => ({
  AppSidebarWorkspacePicker: () => <div data-testid="workspace-picker" />,
}));

import { AppSidebarHeader } from "./app-sidebar-header";

const workspaces = [
  { id: "kanban-1", name: "Kanban", office_workflow_id: "" },
  { id: "office-1", name: "Office", office_workflow_id: "wf-office" },
];

function renderHeader(collapsed = false) {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.workspaces.all(), workspaces);

  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <AppSidebarHeader collapsed={collapsed} onToggleCollapse={vi.fn()} />
      </TooltipProvider>
    </QueryClientProvider>,
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

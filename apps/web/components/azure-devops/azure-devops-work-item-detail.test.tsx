import { afterEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { AzureDevOpsWorkItemDetail } from "./azure-devops-work-item-detail";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

vi.mock("@/hooks/domains/azure-devops/use-azure-devops-work-item-detail", () => ({
  useAzureDevOpsWorkItemDetail: () => ({
    item: {
      id: 101,
      revision: 7,
      title: "Fix build",
      description: "<p>Safe text</p><script>window.bad = true</script>",
      state: "Active",
      type: "Bug",
      assignedTo: "Ada",
      planningFields: [
        { referenceName: "Microsoft.VSTS.Scheduling.Effort", label: "Effort", value: "3" },
      ],
      tags: ["build"],
      webUrl: "https://dev.azure.com/acme/wi/101",
    },
    comments: [{ id: 1, content: "Discussed", author: { id: "u1", displayName: "Grace" } }],
    linkedTasks: [],
    loading: false,
    commentsLoading: false,
    linkedTasksLoading: false,
    error: null,
    commentsError: null,
    continuationToken: undefined,
    refresh: vi.fn(),
    retryComments: vi.fn(),
    loadOlderComments: vi.fn(),
    updateAssignee: vi.fn(),
  }),
}));

describe("AzureDevOpsWorkItemDetail", () => {
  afterEach(() => cleanup());

  it("renders read-only detail, sanitized description, planning fields, discussion, and assignment", () => {
    render(
      <AzureDevOpsWorkItemDetail
        open
        onOpenChange={vi.fn()}
        workspaceId="ws-1"
        projectId="project-1"
        initialItem={{ id: 101, revision: 7, title: "Fix build", state: "Active", type: "Bug" }}
        quickActions={[
          {
            id: "implement",
            label: "Implement",
            hint: "Build it",
            icon: "wrench",
            promptTemplate: "Implement",
          },
        ]}
        onStartTask={vi.fn()}
      />,
    );
    expect(screen.getByTestId("azure-work-item-detail")).toBeTruthy();
    expect(screen.getAllByText("Fix build").length).toBeGreaterThan(0);
    expect(screen.getByText("Effort")).toBeTruthy();
    expect(screen.getByText("Discussed")).toBeTruthy();
    expect(screen.getByTestId("azure-work-item-assign-current-user")).toBeTruthy();
    expect(screen.getByTestId("azure-work-item-quick-actions")).toBeTruthy();
    expect(screen.queryByText("window.bad = true")).toBeNull();
  });

  it("opens when a board selection transitions from closed to open", () => {
    function Harness() {
      const [open, setOpen] = React.useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Open
          </button>
          <AzureDevOpsWorkItemDetail
            open={open}
            onOpenChange={setOpen}
            workspaceId="ws-1"
            projectId="project-1"
            initialItem={{ id: 101, revision: 7, title: "Fix build", state: "Active", type: "Bug" }}
          />
        </>
      );
    }
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(screen.getByTestId("azure-work-item-detail")).toBeTruthy();
  });
});

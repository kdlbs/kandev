import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import React from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AzureDevOpsWorkItemDetail } from "./azure-devops-work-item-detail";
import type { AzureDevOpsWorkItem, AzureDevOpsBoardWorkItem } from "@/lib/types/azure-devops";

const assignButtonTestId = "azure-work-item-assign-current-user";

const mutationOwner = vi.hoisted(() => ({ current: 0, merge: vi.fn(), assign: vi.fn() }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

vi.mock("@/hooks/domains/azure-devops/use-azure-devops-work-item-detail", () => ({
  useAzureDevOpsWorkItemDetail: (
    _workspace: unknown,
    _project: unknown,
    initialItem: AzureDevOpsWorkItem | null,
  ) => ({
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
      ...initialItem,
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
    updateAssignee: mutationOwner.assign,
    beginMutation: () => {
      const owner = mutationOwner.current;
      return { isCurrent: () => owner === mutationOwner.current, mergeItem: mutationOwner.merge };
    },
  }),
}));

beforeEach(() => {
  mutationOwner.current = 0;
  mutationOwner.merge.mockClear();
  mutationOwner.assign.mockReset();
});

afterEach(cleanup);

describe("AzureDevOpsWorkItemDetail", () => {
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
    expect(screen.getByTestId(assignButtonTestId)).toBeTruthy();
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

it.each(["success", "failure"])(
  "keeps a new detail opening busy after old assignment %s",
  async (outcome) => {
    let resolveOld!: (item: AzureDevOpsBoardWorkItem) => void;
    let rejectOld!: (error: Error) => void;
    let resolveCurrent!: (item: AzureDevOpsBoardWorkItem) => void;
    const old = new Promise<AzureDevOpsBoardWorkItem>((resolve, reject) => {
      resolveOld = resolve;
      rejectOld = reject;
    });
    const current = new Promise<AzureDevOpsBoardWorkItem>((resolve) => {
      resolveCurrent = resolve;
    });
    mutationOwner.assign.mockReturnValueOnce(old).mockReturnValueOnce(current);
    const onItemUpdated = vi.fn();
    const first = {
      id: 101,
      revision: 7,
      title: "First",
      state: "Active",
      type: "Task",
      columnId: "todo",
      columnDone: false,
    };
    const second = { ...first, id: 102, title: "Second" };
    const props = (item: AzureDevOpsBoardWorkItem) => ({
      open: true,
      onOpenChange: vi.fn(),
      workspaceId: "ws-1",
      projectId: "project-1",
      initialItem: item,
      boardContext: {
        board: {
          id: "board",
          name: "Board",
          columns: [{ id: "todo", name: "To Do" }],
          fields: {
            columnField: { referenceName: "System.BoardColumn" },
            doneField: { referenceName: "System.BoardColumnDone" },
            rowField: { referenceName: "System.BoardRow" },
          },
        },
        item,
        onMove: vi.fn(),
        onItemUpdated,
      },
    });
    const { rerender } = render(<AzureDevOpsWorkItemDetail {...props(first)} />);
    fireEvent.click(screen.getByTestId(assignButtonTestId));
    mutationOwner.current += 1;
    rerender(<AzureDevOpsWorkItemDetail {...props(second)} />);
    fireEvent.click(screen.getByTestId(assignButtonTestId));
    await act(async () => {
      if (outcome === "success") resolveOld({ ...first, revision: 8 });
      else rejectOld(new Error("Old opening failure"));
    });
    expect(mutationOwner.assign).toHaveBeenCalledTimes(2);
    if (outcome === "success")
      expect(onItemUpdated).toHaveBeenCalledWith(expect.objectContaining({ id: 101, revision: 8 }));
    else expect(onItemUpdated).not.toHaveBeenCalled();
    expect(mutationOwner.merge).not.toHaveBeenCalled();
    expect((screen.getByTestId(assignButtonTestId) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByText("Old opening failure")).toBeNull();
    await act(async () => {
      resolveCurrent({ ...second, revision: 8 });
    });
    await waitFor(() =>
      expect((screen.getByTestId(assignButtonTestId) as HTMLButtonElement).disabled).toBe(false),
    );
    expect(onItemUpdated).toHaveBeenCalledWith(expect.objectContaining({ id: 102, revision: 8 }));
  },
);

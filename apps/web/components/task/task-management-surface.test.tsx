import { useRef } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useStore } from "zustand";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createAppStore, type AppState } from "@/lib/state/store";
import { useTaskManagementFlow } from "@/hooks/use-task-management-flow";
import { TaskManagementSurface } from "./task-management-surface";

const api = vi.hoisted(() => ({
  linkTaskIssue: vi.fn(),
  archiveTask: vi.fn(),
  bulkMoveSelectedTasks: vi.fn(),
  toast: vi.fn(),
  getSubtaskCount: vi.fn(),
}));
const pointer = vi.hoisted(() => ({ isMobile: true, isFinePointer: false }));
const ISSUE_INPUT = "task-github-issue-input";
let store: ReturnType<typeof createAppStore>;
let flow: ReturnType<typeof useTaskManagementFlow>;
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: api.toast }) }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => pointer,
}));
vi.mock("@/lib/api/domains/github-api", async (original) => ({
  ...(await original<typeof import("@/lib/api/domains/github-api")>()),
  linkTaskIssue: api.linkTaskIssue,
}));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  getSubtaskCount: api.getSubtaskCount,
  archiveTask: api.archiveTask,
  bulkMoveSelectedTasks: api.bulkMoveSelectedTasks,
}));
afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  pointer.isMobile = true;
  pointer.isFinePointer = false;
  api.getSubtaskCount.mockResolvedValue({ count: 0 });
  api.bulkMoveSelectedTasks.mockResolvedValue({ moved_count: 1 });
  store = createAppStore();
  store.setState((state) => ({
    workspaces: { ...state.workspaces, activeId: "workspace" },
    workflows: {
      ...state.workflows,
      items: [{ id: "workflow", name: "Flow", workspaceId: "workspace" }],
    },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        workflow: {
          workflowId: "workflow",
          workflowName: "Flow",
          steps: [],
          tasks: ["A", "B"].map((id) => ({
            id,
            title: id,
            workflowId: "workflow",
            workflowStepId: "step",
            position: 0,
          })),
        },
      },
    },
  }));
});

function Harness({ showAnchor = true }: { showAnchor?: boolean }) {
  flow = useTaskManagementFlow();
  const anchorRef = useRef<HTMLButtonElement>(null);
  const fallbackRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      {showAnchor && (
        <button ref={anchorRef} onClick={() => flow.open("A")}>
          Open A
        </button>
      )}
      <button ref={fallbackRef}>Surviving thread</button>
      <TaskManagementSurface
        flow={flow}
        point={{ x: 0, y: 0 }}
        anchorRef={anchorRef}
        focusReturnRef={fallbackRef}
        onReturnFocus={() => {}}
      />
    </>
  );
}

function openIssue() {
  fireEvent.click(screen.getByRole("button", { name: "Link" }));
  fireEvent.click(screen.getByRole("button", { name: "GitHub Issue" }));
}

// @covers AC-TASKS-THREADS-ACTIONS-002.2, AC-TASKS-THREADS-ACTIONS-002.5
it("a late link result for A cannot close or populate a newer link flow for B", async () => {
  let release!: () => void;
  api.linkTaskIssue.mockReturnValueOnce(
    new Promise<void>((resolve) => {
      release = resolve;
    }),
  );
  render(<Harness />);
  fireEvent.click(screen.getByText("Open A"));
  openIssue();
  fireEvent.change(screen.getByTestId(ISSUE_INPUT), {
    target: { value: "https://github.com/test/repo/issues/1" },
  });
  act(() => store.getState().setActiveSession("B", "session-B"));
  fireEvent.click(screen.getByTestId("task-github-issue-submit"));
  expect(api.linkTaskIssue).toHaveBeenCalledWith("A", {
    issue: "https://github.com/test/repo/issues/1",
  });
  act(() => flow.open("B"));
  openIssue();
  expect((screen.getByTestId(ISSUE_INPUT) as HTMLInputElement).value).toBe("");
  await act(async () => release());
  expect((screen.getByTestId(ISSUE_INPUT) as HTMLInputElement).disabled).toBe(false);
  expect(flow.identity?.taskId).toBe("B");
});

// @covers AC-TASKS-THREADS-ACTIONS-002.3
it("discarding an inaccessible target also discards its provider form state", async () => {
  render(<Harness />);
  fireEvent.click(screen.getByText("Open A"));
  openIssue();
  act(() =>
    store.setState((state) => ({ workspaces: { ...state.workspaces, activeId: "elsewhere" } })),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  act(() =>
    store.setState((state) => ({ workspaces: { ...state.workspaces, activeId: "workspace" } })),
  );
  act(() => flow.open("B"));
  fireEvent.click(screen.getByRole("button", { name: "Link" }));
  fireEvent.click(screen.getByRole("button", { name: "GitHub Pull Request" }));
  expect(screen.queryByTestId(ISSUE_INPUT)).toBeNull();
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
});

// @covers AC-TASKS-THREADS-ACTIONS-002.3, AC-TASKS-THREADS-ACTIONS-004.6
it("keeps a pending archive for A when only its header is filtered out", async () => {
  pointer.isMobile = false;
  pointer.isFinePointer = true;
  let release!: (value: { count: number }) => void;
  api.getSubtaskCount.mockReturnValueOnce(
    new Promise((resolve) => {
      release = resolve;
    }),
  );
  const view = render(<Harness />);
  fireEvent.click(screen.getByText("Open A"));
  act(() => flow.setStage("archive"));
  await waitFor(() => expect(api.getSubtaskCount).toHaveBeenCalledWith("A"));
  view.rerender(<Harness showAnchor={false} />);
  expect(flow.stage).toBe("archive");
  await act(async () => release({ count: 0 }));
  expect(await screen.findByTestId("archive-task-confirm")).toBeTruthy();
  expect(flow.getTarget()?.id).toBe("A");
});

// @covers AC-TASKS-THREADS-ACTIONS-001.3, AC-TASKS-THREADS-ACTIONS-002.2
it("allows moves within the current hidden workflow without offering other hidden workflows", async () => {
  store.setState((state) => ({
    workflows: {
      ...state.workflows,
      items: ["workflow", "other-hidden"].map((id) => ({
        id,
        name: id,
        workspaceId: "workspace",
        hidden: true,
      })),
    },
    kanbanMulti: {
      ...state.kanbanMulti,
      snapshots: {
        workflow: {
          ...state.kanbanMulti.snapshots.workflow,
          steps: ["step", "next"].map((id, position) => ({
            id,
            title: id,
            position,
            color: "",
          })),
        },
      },
    },
  }));
  render(<Harness />);
  fireEvent.click(screen.getByText("Open A"));
  expect(screen.queryByRole("button", { name: "Send to workflow" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Move to" }));
  fireEvent.click(screen.getByRole("button", { name: "next" }));
  await waitFor(() =>
    expect(api.bulkMoveSelectedTasks).toHaveBeenCalledWith({
      task_ids: ["A"],
      target_workflow_id: "workflow",
      target_step_id: "next",
    }),
  );
});

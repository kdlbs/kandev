import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTaskRemovalState } from "@/lib/state/task-removal";

const bootstrapMock = vi.hoisted(() => vi.fn());
const searchMock = vi.hoisted(() => ({ value: "" }));
const snapshotsMock = vi.hoisted(() => vi.fn());
const snapshotReadinessMock = vi.hoisted(() => ({ ready: true }));
const threadsBoardMock = vi.hoisted(() => vi.fn());
const appStateMock = vi.hoisted(() => ({
  snapshots: {} as Record<string, unknown>,
  isLoading: false,
  threadViews: undefined as unknown,
  taskRemoval: undefined as unknown,
}));
const displaySettingsMock = vi.hoisted(() => ({
  activeWorkspaceId: "stored-workspace" as string | null,
  activeWorkflowId: null as string | null,
  workspaces: [] as Array<{ id: string }>,
  workflows: [] as Array<{ id: string; workspaceId: string }>,
}));
const WORKSPACE_ID = "workspace-1";

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(searchMock.value),
}));
vi.mock("@/src/kanban-route", () => ({ useKanbanRouteBootstrap: bootstrapMock }));
vi.mock("@/components/kanban/kanban-header", () => ({ KanbanHeader: () => null }));
vi.mock("@/components/threads/threads-board", () => ({
  ThreadsBoard: (props: { threads: unknown[] }) => {
    threadsBoardMock(props);
    return null;
  },
}));
vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: (...args: unknown[]) => {
    snapshotsMock(...args);
    return {
      refresh: vi.fn(),
      initialSnapshotsReady: snapshotReadinessMock.ready,
    };
  },
}));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => displaySettingsMock,
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ setView: vi.fn() }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      kanbanMulti: { snapshots: appStateMock.snapshots, isLoading: appStateMock.isLoading },
      threadViews: appStateMock.threadViews,
      taskRemoval: appStateMock.taskRemoval,
    }),
}));

import { DEFAULT_THREAD_VIEW } from "@/lib/state/slices/ui/thread-view-builtins";
import type { KanbanState, WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { scopeSnapshotsToWorkspace, ThreadsPageClient } from "./threads-page-client";

function threadTask(id: string, workflowId: string): KanbanState["tasks"][number] {
  return {
    id,
    workspaceId: WORKSPACE_ID,
    workflowId,
    workflowStepId: `${workflowId}-step`,
    title: `Task ${id}`,
    position: 0,
    state: "IN_PROGRESS",
    primarySessionId: `session-${id}`,
    primarySessionState: "RUNNING",
    updatedAt: "2026-08-31T10:00:00Z",
    createdAt: "2026-08-30T10:00:00Z",
  };
}

function workflowSnapshot(workflowId: string, taskId: string): WorkflowSnapshotData {
  return {
    workflowId,
    workflowName: workflowId,
    steps: [{ id: `${workflowId}-step`, title: "Build", color: "blue", position: 0 }],
    tasks: [threadTask(taskId, workflowId)],
  };
}

afterEach(() => {
  cleanup();
  bootstrapMock.mockReset();
  snapshotsMock.mockReset();
  threadsBoardMock.mockReset();
  searchMock.value = "";
});

beforeEach(() => {
  snapshotsMock.mockImplementation(() => ({ refresh: vi.fn() }));
  snapshotReadinessMock.ready = true;
  threadsBoardMock.mockReset();
  appStateMock.snapshots = {};
  appStateMock.isLoading = false;
  appStateMock.threadViews = undefined;
  appStateMock.taskRemoval = createTaskRemovalState();
  displaySettingsMock.workspaces = [];
  displaySettingsMock.workflows = [];
});

describe("ThreadsPageClient — workspace deep links", () => {
  it("bootstraps the workspace the link asked for, not the stored one", () => {
    searchMock.value = "workspace=requested-workspace";

    render(<ThreadsPageClient />);

    expect(bootstrapMock).toHaveBeenCalledWith(
      expect.objectContaining({ workspaceId: "requested-workspace" }),
      false,
    );
  });

  it("uses the stored workspace when the link names an unknown workspace", () => {
    displaySettingsMock.workspaces = [{ id: "ws-a" }];
    searchMock.value = "workspace=UNKNOWN";

    render(<ThreadsPageClient />);

    expect(snapshotsMock).toHaveBeenCalledWith("stored-workspace");
  });

  it("scopes snapshots to the requested workspace before effects clear stale data", () => {
    const snapshots = {
      "wf-old": { workflowId: "wf-old", workflowName: "Old", steps: [], tasks: [] },
      "wf-new": { workflowId: "wf-new", workflowName: "New", steps: [], tasks: [] },
    };

    expect(
      Object.keys(
        scopeSnapshotsToWorkspace(
          snapshots,
          [
            { id: "wf-old", workspaceId: "workspace-old" },
            { id: "wf-new", workspaceId: "workspace-new" },
          ],
          "workspace-new",
        ),
      ),
    ).toEqual(["wf-new"]);
  });

  it("leaves the workspace unset when the link names none", () => {
    render(<ThreadsPageClient />);

    expect(bootstrapMock).toHaveBeenCalledWith(
      expect.objectContaining({ workspaceId: undefined }),
      false,
    );
  });

  it("keeps one bootstrap route identity across re-renders of the same link", () => {
    searchMock.value = "workspace=requested-workspace";
    const { rerender } = render(<ThreadsPageClient />);
    rerender(<ThreadsPageClient />);

    const [first] = bootstrapMock.mock.calls[0];
    const [second] = bootstrapMock.mock.calls[bootstrapMock.mock.calls.length - 1];
    expect(first).toBe(second);
  });

  it("reapplies saved selection order after initial workflow snapshots settle", () => {
    displaySettingsMock.activeWorkspaceId = WORKSPACE_ID;
    displaySettingsMock.workspaces = [{ id: WORKSPACE_ID }];
    displaySettingsMock.workflows = [
      { id: "workflow-a", workspaceId: WORKSPACE_ID },
      { id: "workflow-b", workspaceId: WORKSPACE_ID },
    ];
    appStateMock.threadViews = {
      views: [
        {
          ...DEFAULT_THREAD_VIEW,
          id: "selected-view",
          taskScope: { mode: "selected", taskIds: ["task-a", "task-b"] },
          sort: { key: "selection", direction: "asc" },
          maxColumns: 1,
        },
      ],
      activeViewId: "selected-view",
      draft: null,
      orderResetGeneration: 0,
    };
    snapshotReadinessMock.ready = false;
    appStateMock.snapshots = { "workflow-b": workflowSnapshot("workflow-b", "task-b") };

    const { rerender } = render(<ThreadsPageClient />);
    const initialThreads = threadsBoardMock.mock.lastCall?.[0].threads as Array<{
      taskId: string;
    }>;
    expect(initialThreads.map(({ taskId }) => taskId)).toEqual(["task-b"]);

    appStateMock.snapshots = {
      "workflow-b": workflowSnapshot("workflow-b", "task-b"),
      "workflow-a": workflowSnapshot("workflow-a", "task-a"),
    };
    snapshotReadinessMock.ready = true;
    rerender(<ThreadsPageClient />);

    const settledThreads = threadsBoardMock.mock.lastCall?.[0].threads as Array<{
      taskId: string;
    }>;
    expect(settledThreads.map(({ taskId }) => taskId)).toEqual(["task-a"]);
  });
});

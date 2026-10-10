import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockListWorkflows = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();
const mockListWorkspaceTaskPRs = vi.fn();

const ROUTE_WORKSPACE_ID = "ws-route";
const ACTIVE_WORKSPACE_ID = "ws-active-elsewhere";
const NOW_ISO = "2026-09-27T00:00:00Z";

type MockState = {
  workspaces: { activeId: string | null };
  workflows: { items: never[] };
  kanbanMulti: { snapshots: Record<string, unknown> };
  workspaceContextRead: {
    workspaceId: string | null;
    snapshotPending: boolean;
    snapshotError: string | null;
    snapshotRequestId: string | null;
  };
  requestWorkspaceContextRefresh: () => void;
  taskPRs: { workspaceId: string | null; byTaskId: Record<string, unknown> };
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

// The active-cache builders are only ever invoked with `null` in this
// scenario (route workspace != active workspace) — stub them as no-ops so
// this test exercises only the direct-fetch path, without needing the full
// shared-store contract those builders depend on.
vi.mock("@/hooks/domains/kanban/use-all-workflow-snapshots", () => ({
  useAllWorkflowSnapshots: () => undefined,
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useWorkspacePRs: () => undefined,
}));

vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => null,
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listWorkflows: (...args: unknown[]) => mockListWorkflows(...args),
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  listWorkspaceTaskPRs: (...args: unknown[]) => mockListWorkspaceTaskPRs(...args),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => false }));
vi.mock("./use-coordinator-watch-set", () => ({
  useCoordinatorWatchSet: () => ({
    input: { value: undefined, error: false, loadedAt: undefined },
    retry: vi.fn(),
  }),
}));

import { useCoordinatorAttention } from "./use-coordinator-attention";

beforeEach(() => {
  vi.clearAllMocks();
  mockState = {
    workspaces: { activeId: ACTIVE_WORKSPACE_ID },
    workflows: { items: [] },
    kanbanMulti: { snapshots: {} },
    workspaceContextRead: {
      workspaceId: null,
      snapshotPending: false,
      snapshotError: null,
      snapshotRequestId: null,
    },
    requestWorkspaceContextRefresh: vi.fn(),
    taskPRs: { workspaceId: null, byTaskId: {} },
  };
});

// Build round 4: the route's workspace can differ from the globally active
// one (a coordinator deep link, or a cold load into a workspace that isn't
// active yet). Every coordinator data hook must read correctly for the
// *route's* workspaceId regardless of what's globally active — this is the
// fix for the R2 -> R4 -> R5 fix-induced chain (build round 4's redirect).
describe("useCoordinatorAttention - deep link to a non-active workspace", () => {
  it("classifies Needs you and Queue, including Queue's PR detail, for the route's own workspace", async () => {
    const needsYouTask = {
      id: "t-question",
      workspace_id: ROUTE_WORKSPACE_ID,
      workflow_id: "wf-1",
      workflow_step_id: "step-1",
      position: 0,
      title: "Needs a decision",
      description: "",
      state: "IN_PROGRESS",
      priority: "medium",
      created_at: NOW_ISO,
      updated_at: NOW_ISO,
      status_summary: {
        revision: 1,
        updated_at: NOW_ISO,
        last_activity_at: NOW_ISO,
        pending_action: "clarification",
      },
    };
    const queueTask = {
      id: "t-review",
      workspace_id: ROUTE_WORKSPACE_ID,
      workflow_id: "wf-1",
      workflow_step_id: "step-1",
      position: 1,
      title: "In review",
      description: "",
      state: "IN_PROGRESS",
      priority: "medium",
      created_at: NOW_ISO,
      updated_at: NOW_ISO,
      status_summary: {
        revision: 1,
        updated_at: NOW_ISO,
        last_activity_at: NOW_ISO,
        pull_request: { aggregate_state: "awaiting_review" },
      },
    };

    mockListWorkflows.mockResolvedValue({
      workflows: [
        {
          id: "wf-1",
          workspace_id: ROUTE_WORKSPACE_ID,
          name: "Planner",
          created_at: NOW_ISO,
          updated_at: NOW_ISO,
        },
      ],
      total: 1,
    });
    mockFetchWorkflowSnapshot.mockResolvedValue({
      workflow: {
        id: "wf-1",
        workspace_id: ROUTE_WORKSPACE_ID,
        name: "Planner",
        created_at: NOW_ISO,
        updated_at: NOW_ISO,
      },
      steps: [
        {
          id: "step-1",
          workflow_id: "wf-1",
          name: "Build",
          position: 0,
          color: "bg-neutral-400",
          allow_manual_move: true,
          complete_task_on_enter: false,
          auto_advance_requires_signal: false,
          cancel_triggers_turn_complete: false,
        },
      ],
      tasks: [needsYouTask, queueTask],
    });
    mockListWorkspaceTaskPRs.mockResolvedValue({
      task_prs: {
        "t-review": [
          { id: "pr-1", state: "open", unresolved_review_threads: 1, checks_state: "pending" },
        ],
      },
    });

    const { result } = renderHook(() => useCoordinatorAttention(ROUTE_WORKSPACE_ID, null));

    await waitFor(() => expect(result.current.tasksNeverLoaded).toBe(false));

    expect(mockListWorkflows).toHaveBeenCalledWith(ROUTE_WORKSPACE_ID);
    expect(mockListWorkspaceTaskPRs).toHaveBeenCalledWith(ROUTE_WORKSPACE_ID, {
      cache: "no-store",
    });

    expect(result.current.classification.needsYou.map((item) => item.id)).toEqual(["t-question"]);
    expect(result.current.classification.queue.in_review.map((item) => item.id)).toEqual([
      "t-review",
    ]);

    const pr = result.current.prsByTaskId.get("t-review")?.[0];
    expect(pr?.state).toBe("open");
    expect(pr?.unresolved_review_threads).toBe(1);
  });
});

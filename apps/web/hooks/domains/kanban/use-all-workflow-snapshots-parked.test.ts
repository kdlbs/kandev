import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

// Testing round (post-Build-round-3): a REST-driven kanban board refresh
// (foreground tab refocus, WS reconnect) can resolve after a fresher
// task.updated already applied a newer parked triple to the existing
// snapshot. Before this fix, fetchAndWriteSnapshot took the REST response's
// parked_on_background_work/parked_epoch/parked_revision verbatim, with zero
// staleness comparison — D1's (parked_epoch, revision) discard rule (AC-39,
// AC-49) applies to every consumer, not only WS handlers. Split into its own
// file (rather than use-all-workflow-snapshots.test.ts) to keep that file's
// "snapshot mapping" describe block under its 100-line function limit.

const mockClearKanbanMulti = vi.fn();
const mockSetKanbanMultiLoading = vi.fn();
const mockSetWorkflowSnapshot = vi.fn();
const mockFetchWorkflowSnapshot = vi.fn();

type Workflow = { id: string; workspaceId: string; name: string };
type MockState = {
  connection: { status: string };
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
  workflows: { items: Workflow[] };
  kanbanMulti: { snapshots: Record<string, unknown>; isLoading: boolean };
  clearKanbanMulti: typeof mockClearKanbanMulti;
  setKanbanMultiLoading: typeof mockSetKanbanMultiLoading;
  setWorkflowSnapshot: typeof mockSetWorkflowSnapshot;
};

let mockState: MockState = {
  connection: { status: "connected" },
  workspaces: { activeId: "ws-A" },
  workspaceContextGeneration: 0,
  workflows: { items: [] },
  kanbanMulti: { snapshots: {}, isLoading: false },
  clearKanbanMulti: mockClearKanbanMulti,
  setKanbanMultiLoading: mockSetKanbanMultiLoading,
  setWorkflowSnapshot: mockSetWorkflowSnapshot,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
  useAppStoreApi: () => ({ getState: () => mockState }),
}));

vi.mock("@/lib/api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => mockFetchWorkflowSnapshot(...args),
}));

import { useAllWorkflowSnapshots } from "./use-all-workflow-snapshots";

function resetMocks(workflows: Workflow[] = []) {
  vi.clearAllMocks();
  mockFetchWorkflowSnapshot.mockResolvedValue({ steps: [], tasks: [] });
  mockState = {
    connection: { status: "connected" },
    workspaces: { activeId: workflows[0]?.workspaceId ?? null },
    workspaceContextGeneration: 0,
    workflows: { items: workflows },
    kanbanMulti: { snapshots: {}, isLoading: false },
    clearKanbanMulti: mockClearKanbanMulti,
    setKanbanMultiLoading: mockSetKanbanMultiLoading,
    setWorkflowSnapshot: mockSetWorkflowSnapshot,
  };
}

describe("useAllWorkflowSnapshots — parked projection ordering (D1, AC-39, AC-49)", () => {
  beforeEach(() => {
    resetMocks([{ id: "wf-A", workspaceId: "ws-A", name: "A" }]);
  });

  it("discards a stale parked triple carried on a REST snapshot task", async () => {
    mockState.kanbanMulti.snapshots = {
      "wf-A": {
        workflowId: "wf-A",
        workflowName: "A",
        steps: [{ id: "step-1" }],
        tasks: [
          {
            id: "t1",
            workflowStepId: "step-1",
            parkedOnBackgroundWork: true,
            parkedEpoch: 100,
            parkedRevision: 7,
          },
        ],
      },
    };
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1, color: "bg-blue-500" }],
      tasks: [
        {
          id: "t1",
          workflow_step_id: "step-1",
          title: "Task",
          parked_on_background_work: false,
          parked_epoch: 100,
          parked_revision: 6,
        },
      ],
    });

    renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );
    // Snapshot is already boot-hydrated, so the hook skips the initial-mount
    // fetch. Force the same refetch path a tab foreground-refocus triggers
    // in production.
    act(() => window.dispatchEvent(new Event("focus")));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0]).toMatchObject({
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 7,
    });
  });

  it("accepts a fresher parked triple carried on a REST snapshot task", async () => {
    mockState.kanbanMulti.snapshots = {
      "wf-A": {
        workflowId: "wf-A",
        workflowName: "A",
        steps: [{ id: "step-1" }],
        tasks: [
          {
            id: "t1",
            workflowStepId: "step-1",
            parkedOnBackgroundWork: false,
            parkedEpoch: 100,
            parkedRevision: 1,
          },
        ],
      },
    };
    mockFetchWorkflowSnapshot.mockResolvedValueOnce({
      steps: [{ id: "step-1", name: "Review", position: 1, color: "bg-blue-500" }],
      tasks: [
        {
          id: "t1",
          workflow_step_id: "step-1",
          title: "Task",
          parked_on_background_work: true,
          parked_epoch: 100,
          parked_revision: 2,
        },
      ],
    });

    renderHook(
      ({ workspaceId }: { workspaceId: string | null }) => useAllWorkflowSnapshots(workspaceId),
      { initialProps: { workspaceId: "ws-A" } },
    );
    act(() => window.dispatchEvent(new Event("focus")));

    await waitFor(() => expect(mockSetWorkflowSnapshot).toHaveBeenCalled());
    expect(mockSetWorkflowSnapshot.mock.calls[0][1].tasks[0]).toMatchObject({
      parkedOnBackgroundWork: true,
      parkedEpoch: 100,
      parkedRevision: 2,
    });
  });
});

import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Repository, Workflow, WorkflowStepDTO } from "@/lib/types/http";

const listWorkflowsMock = vi.fn();
const fetchWorkflowSnapshotMock = vi.fn();
const listRepositoriesMock = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listWorkflows: (...args: unknown[]) => listWorkflowsMock(...args),
  fetchWorkflowSnapshot: (...args: unknown[]) => fetchWorkflowSnapshotMock(...args),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositories: (...args: unknown[]) => listRepositoriesMock(...args),
}));

import { useProposalEditOptions } from "./use-proposal-edit-options";

const WORKSPACE_ID = "w-1";

function workflow(id: string, name: string): Workflow {
  return { id, workspace_id: WORKSPACE_ID, name } as Workflow;
}

function step(overrides: Partial<WorkflowStepDTO> & { id: string }): WorkflowStepDTO {
  return {
    workflow_id: "wf-1",
    name: overrides.id,
    position: 0,
    color: "#000",
    allow_manual_move: false,
    complete_task_on_enter: false,
    auto_advance_requires_signal: false,
    cancel_triggers_turn_complete: false,
    ...overrides,
  } as WorkflowStepDTO;
}

function repository(id: string, name: string): Repository {
  return { id, workspace_id: WORKSPACE_ID, name } as Repository;
}

beforeEach(() => {
  listWorkflowsMock.mockReset();
  fetchWorkflowSnapshotMock.mockReset();
  listRepositoriesMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("useProposalEditOptions", () => {
  it("loads workflows and repositories once, and the current workflow's eligible steps", async () => {
    listWorkflowsMock.mockResolvedValue({ workflows: [workflow("wf-1", "Build")], total: 1 });
    listRepositoriesMock.mockResolvedValue({ repositories: [repository("repo-1", "app")] });
    fetchWorkflowSnapshotMock.mockResolvedValue({
      workflow: workflow("wf-1", "Build"),
      steps: [
        step({ id: "start", is_start_step: true, allow_manual_move: true }),
        step({ id: "locked", is_start_step: false, allow_manual_move: false }),
      ],
      tasks: [],
    });

    const { result } = renderHook(() => useProposalEditOptions(WORKSPACE_ID, "wf-1"));

    await waitFor(() => expect(result.current.workflows.status).toBe("loaded"));
    await waitFor(() => expect(result.current.repositories.status).toBe("loaded"));
    await waitFor(() => expect(result.current.steps.status).toBe("loaded"));

    expect(result.current.workflows.value).toEqual([workflow("wf-1", "Build")]);
    expect(result.current.repositories.value).toEqual([repository("repo-1", "app")]);
    expect(result.current.steps.value.map((s) => s.id)).toEqual(["start"]);
    expect(listWorkflowsMock).toHaveBeenCalledTimes(1);
    expect(listRepositoriesMock).toHaveBeenCalledTimes(1);
  });

  it("re-reads the step snapshot when workflowId changes, and discards a stale response", async () => {
    listWorkflowsMock.mockResolvedValue({ workflows: [], total: 0 });
    listRepositoriesMock.mockResolvedValue({ repositories: [] });

    let resolveFirst!: (v: unknown) => void;
    fetchWorkflowSnapshotMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );
    fetchWorkflowSnapshotMock.mockImplementationOnce(() =>
      Promise.resolve({
        workflow: workflow("wf-2", "Ship"),
        steps: [step({ id: "s2", is_start_step: true, allow_manual_move: true })],
        tasks: [],
      }),
    );

    const { result, rerender } = renderHook(
      ({ workflowId }: { workflowId: string }) => useProposalEditOptions(WORKSPACE_ID, workflowId),
      { initialProps: { workflowId: "wf-1" } },
    );

    rerender({ workflowId: "wf-2" });
    await waitFor(() => expect(result.current.steps.status).toBe("loaded"));
    expect(result.current.steps.value.map((s) => s.id)).toEqual(["s2"]);

    // The stale first request resolving afterwards must not overwrite wf-2's result.
    resolveFirst({
      workflow: workflow("wf-1", "Build"),
      steps: [step({ id: "s1", is_start_step: true, allow_manual_move: true })],
      tasks: [],
    });
    await Promise.resolve();
    expect(result.current.steps.value.map((s) => s.id)).toEqual(["s2"]);
  });

  it("sets an error status when a read fails, and retry re-issues it", async () => {
    listWorkflowsMock.mockRejectedValueOnce(new Error("boom"));
    listWorkflowsMock.mockResolvedValueOnce({ workflows: [workflow("wf-1", "Build")], total: 1 });
    listRepositoriesMock.mockResolvedValue({ repositories: [] });
    fetchWorkflowSnapshotMock.mockResolvedValue({
      workflow: workflow("wf-1", "Build"),
      steps: [],
      tasks: [],
    });

    const { result } = renderHook(() => useProposalEditOptions(WORKSPACE_ID, "wf-1"));

    await waitFor(() => expect(result.current.workflows.status).toBe("error"));

    result.current.retryWorkflows();
    await waitFor(() => expect(result.current.workflows.status).toBe("loaded"));
    expect(result.current.workflows.value).toEqual([workflow("wf-1", "Build")]);
  });
});

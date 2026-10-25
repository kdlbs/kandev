import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const fetchWorkflowSnapshotMock = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchWorkflowSnapshot: (...args: unknown[]) => fetchWorkflowSnapshotMock(...args),
}));

import { useProposalWorkflowNames } from "./use-proposal-workflow-names";

beforeEach(() => {
  fetchWorkflowSnapshotMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("useProposalWorkflowNames", () => {
  it("returns empty maps when workflowId is null, and issues no read", () => {
    const { result } = renderHook(() => useProposalWorkflowNames(null));
    expect(result.current.workflowNameById.size).toBe(0);
    expect(result.current.stepNameByWorkflowStep.size).toBe(0);
    expect(fetchWorkflowSnapshotMock).not.toHaveBeenCalled();
  });

  it("resolves the workflow and step names from the one workflow's snapshot", async () => {
    fetchWorkflowSnapshotMock.mockResolvedValue({
      workflow: { id: "wf-1", name: "Build" },
      steps: [{ id: "step-1", name: "Review" }],
      tasks: [],
    });

    const { result } = renderHook(() => useProposalWorkflowNames("wf-1"));

    await waitFor(() => expect(result.current.workflowNameById.get("wf-1")).toBe("Build"));
    expect(result.current.stepNameByWorkflowStep.get("wf-1:step-1")).toBe("Review");
    expect(fetchWorkflowSnapshotMock).toHaveBeenCalledWith("wf-1", { cache: "no-store" });
  });

  it("falls back to the raw id when the snapshot has no workflow name", async () => {
    fetchWorkflowSnapshotMock.mockResolvedValue({
      workflow: undefined,
      steps: [],
      tasks: [],
    });

    const { result } = renderHook(() => useProposalWorkflowNames("wf-1"));

    await waitFor(() => expect(result.current.workflowNameById.get("wf-1")).toBe("wf-1"));
  });

  it("leaves the maps empty when the read fails", async () => {
    fetchWorkflowSnapshotMock.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useProposalWorkflowNames("wf-1"));

    await waitFor(() => expect(fetchWorkflowSnapshotMock).toHaveBeenCalled());
    expect(result.current.workflowNameById.size).toBe(0);
    expect(result.current.stepNameByWorkflowStep.size).toBe(0);
  });

  it("discards a stale response when workflowId changes before it resolves", async () => {
    let resolveFirst: ((value: unknown) => void) | undefined;
    fetchWorkflowSnapshotMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );
    fetchWorkflowSnapshotMock.mockImplementationOnce(() =>
      Promise.resolve({ workflow: { id: "wf-2", name: "Ship" }, steps: [], tasks: [] }),
    );

    const { result, rerender } = renderHook(
      ({ workflowId }: { workflowId: string }) => useProposalWorkflowNames(workflowId),
      { initialProps: { workflowId: "wf-1" } },
    );

    rerender({ workflowId: "wf-2" });
    await waitFor(() => expect(result.current.workflowNameById.get("wf-2")).toBe("Ship"));

    resolveFirst?.({ workflow: { id: "wf-1", name: "Build" }, steps: [], tasks: [] });
    await Promise.resolve();

    expect(result.current.workflowNameById.get("wf-2")).toBe("Ship");
    expect(result.current.workflowNameById.has("wf-1")).toBe(false);
  });
});

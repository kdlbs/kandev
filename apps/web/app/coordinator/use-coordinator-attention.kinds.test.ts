import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Proposal } from "@/lib/api/domains/coordinator-api";
import { useProposalsStore } from "@/hooks/domains/coordinator/use-proposals";

const flag = vi.hoisted(() => ({ phase2: false }));
const inputsMock = vi.hoisted(() => vi.fn());

vi.mock("./use-coordinator-tasks", () => ({
  useCoordinatorTasks: () => ({
    tasks: [],
    stepNameByTaskId: new Map(),
    workflowNameById: new Map(),
    stepNameByWorkflowStep: new Map(),
    error: false,
    loadedAt: 500,
    retry: vi.fn(),
  }),
}));
vi.mock("./use-coordinator-prs", () => ({ useCoordinatorPRs: () => new Map() }));
vi.mock("./use-coordinator-inputs", () => ({
  useCoordinatorInputs: (...args: unknown[]) => inputsMock(...args),
}));
vi.mock("./use-now-tick", () => ({ useNowTick: () => 1_000 }));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => flag.phase2 }));
vi.mock("./use-coordinator-watch-set", () => ({
  useCoordinatorWatchSet: () => ({
    input: { value: undefined, error: false, loadedAt: undefined },
    retry: vi.fn(),
  }),
}));

import { useCoordinatorAttention } from "./use-coordinator-attention";

const WORKSPACE_ID = "workspace-1";
const COORDINATOR_ID = "coordinator-1";

function row(id: string, kind: string | undefined, spec: Record<string, unknown>): Proposal {
  return {
    id,
    coordinator_id: COORDINATOR_ID,
    workspace_id: WORKSPACE_ID,
    status: "pending",
    ...(kind ? { kind } : {}),
    spec,
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
  } as unknown as Proposal;
}

function seed(incoming: Proposal): void {
  const store = useProposalsStore.getState();
  const seq = store.takeProposalTicket(COORDINATOR_ID);
  store.applyProposalResult(COORDINATOR_ID, incoming.id, seq, {
    kind: "success",
    proposal: incoming,
  });
}

beforeEach(() => {
  flag.phase2 = false;
  inputsMock.mockReset().mockReturnValue({
    stalls: { value: undefined, loadedAt: undefined, error: false },
    proposals: { value: undefined, loadedAt: undefined, error: false },
    retryFailed: vi.fn(),
  });
  seed(
    row("c-1", undefined, { title: "t", description: "d", rationale: "r", source_task_id: "t-1" }),
  );
  seed(row("r-1", "resume", { task_id: "t-1", rationale: "r" }));
});
afterEach(() => {
  useProposalsStore.setState({ byCoordinator: {} });
});

describe("useCoordinatorAttention - proposal kinds", () => {
  it("counts only the create_task proposal with the phase-2 flag off", () => {
    const { result } = renderHook(() => useCoordinatorAttention(WORKSPACE_ID, COORDINATOR_ID));
    expect(result.current.computeNeedsYouCount()).toBe(1);
    expect(inputsMock).toHaveBeenCalledWith(WORKSPACE_ID, COORDINATOR_ID, false);
  });

  it("counts every open kind with the phase-2 flag on", () => {
    flag.phase2 = true;
    const { result } = renderHook(() => useCoordinatorAttention(WORKSPACE_ID, COORDINATOR_ID));
    expect(result.current.computeNeedsYouCount()).toBe(2);
    expect(inputsMock).toHaveBeenCalledWith(WORKSPACE_ID, COORDINATOR_ID, true);
  });
});

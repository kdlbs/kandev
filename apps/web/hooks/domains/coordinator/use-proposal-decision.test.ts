import { act, renderHook, waitFor, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Proposal, ProposalSpec } from "@/lib/api/domains/coordinator-api";

const approveProposalMock = vi.fn();
const rejectProposalMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/coordinator-api")>(
    "@/lib/api/domains/coordinator-api",
  );
  return {
    ...actual,
    approveProposal: (...args: unknown[]) => approveProposalMock(...args),
    rejectProposal: (...args: unknown[]) => rejectProposalMock(...args),
  };
});

import { useProposalDecision } from "./use-proposal-decision";
import { useProposalsStore } from "./use-proposals";

const WORKSPACE_ID = "w-1";
const COORDINATOR_ID = "c-1";
const PROPOSAL_ID = "p-1";

function spec(overrides: Partial<ProposalSpec> = {}): ProposalSpec {
  return {
    title: "Add tests",
    description: "desc",
    rationale: "rationale",
    workflow_id: "wf-1",
    step_id: "step-1",
    repository_id: "repo-1",
    source_task_id: "t-1",
    ...overrides,
  };
}

function proposal(overrides: Partial<Proposal> = {}): Proposal {
  return {
    id: PROPOSAL_ID,
    coordinator_id: COORDINATOR_ID,
    workspace_id: WORKSPACE_ID,
    status: "pending",
    spec: spec(),
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

/** Seeds the store as a real caller would: through a ticket, not a shortcut around it. */
function seedProposal(coordinatorId: string, incoming: Proposal): void {
  const store = useProposalsStore.getState();
  const seq = store.takeProposalTicket(coordinatorId);
  store.applyProposalResult(coordinatorId, incoming.id, seq, {
    kind: "success",
    proposal: incoming,
  });
}

beforeEach(() => {
  useProposalsStore.setState({ byCoordinator: {} });
  approveProposalMock.mockReset();
  rejectProposalMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("useProposalDecision - busy state", () => {
  it("sets busy while the request is in flight and clears it after", async () => {
    let resolveApprove!: (p: Proposal) => void;
    approveProposalMock.mockReturnValue(
      new Promise<Proposal>((resolve) => {
        resolveApprove = resolve;
      }),
    );
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcomePromise!: ReturnType<typeof result.current.approve>;
    act(() => {
      outcomePromise = result.current.approve();
    });
    expect(result.current.busy).toBe(true);

    resolveApprove(proposal({ status: "approved", updated_at: "2026-09-28T00:00:00Z" }));
    await act(async () => {
      await outcomePromise;
    });
    expect(result.current.busy).toBe(false);
  });
});

describe("useProposalDecision - decided outcomes", () => {
  it("merges the decided row into the store and returns a decided outcome", async () => {
    const decided = proposal({ status: "approved", updated_at: "2026-09-28T00:00:00Z" });
    approveProposalMock.mockResolvedValue(decided);
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.approve>>;
    await act(async () => {
      outcome = await result.current.approve();
    });

    expect(outcome).toEqual({ kind: "decided", proposal: decided });
    expect(useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId[PROPOSAL_ID]).toEqual(
      decided,
    );
  });

  it("passes edits through to approveProposal", async () => {
    approveProposalMock.mockResolvedValue(proposal({ status: "approved" }));
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    await act(async () => {
      await result.current.approve({ title: "New title" });
    });

    expect(approveProposalMock).toHaveBeenCalledWith(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, {
      title: "New title",
    });
  });

  it("passes a reason through to rejectProposal", async () => {
    rejectProposalMock.mockResolvedValue(proposal({ status: "rejected" }));
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    await act(async () => {
      await result.current.reject("Not now");
    });

    expect(rejectProposalMock).toHaveBeenCalledWith(
      WORKSPACE_ID,
      COORDINATOR_ID,
      PROPOSAL_ID,
      "Not now",
    );
  });
});

describe("useProposalDecision - validation outcomes", () => {
  it("returns a validation outcome for a 400 and does not touch the store", async () => {
    approveProposalMock.mockRejectedValue(
      new ApiError("bad request", 400, { error: "Enter a title", field: "title" }),
    );
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.approve>>;
    await act(async () => {
      outcome = await result.current.approve();
    });

    expect(outcome).toEqual({ kind: "validation", message: "Enter a title", field: "title" });
    expect(useProposalsStore.getState().byCoordinator[COORDINATOR_ID]).toBeUndefined();
  });

  it("returns a validation outcome with a null field when the body names none", async () => {
    approveProposalMock.mockRejectedValue(new ApiError("bad request", 400, { error: "Bad" }));
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.approve>>;
    await act(async () => {
      outcome = await result.current.approve();
    });

    expect(outcome).toEqual({ kind: "validation", message: "Bad", field: null });
  });
});

describe("useProposalDecision - conflict outcomes", () => {
  it("merges the embedded row and returns a conflict outcome for a 409 proposal_conflict", async () => {
    const embedded = proposal({ status: "approving", updated_at: "2026-09-28T00:00:00Z" });
    approveProposalMock.mockRejectedValue(
      new ApiError("conflict", 409, {
        error: "proposal_conflict",
        error_code: "proposal_conflict",
        proposal: embedded,
      }),
    );
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.approve>>;
    await act(async () => {
      outcome = await result.current.approve();
    });

    expect(outcome).toEqual({ kind: "conflict", proposal: embedded });
    expect(useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId[PROPOSAL_ID]).toEqual(
      embedded,
    );
  });

  it("falls back to a network outcome for a 409 with a different error_code", async () => {
    approveProposalMock.mockRejectedValue(
      new ApiError("conflict", 409, { error: "other", error_code: "some_other_conflict" }),
    );
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.approve>>;
    await act(async () => {
      outcome = await result.current.approve();
    });

    expect(outcome).toEqual({ kind: "network" });
  });
});

describe("useProposalDecision - forbidden and not_found outcomes", () => {
  it("returns a forbidden outcome for a 403 and does not touch the store", async () => {
    approveProposalMock.mockRejectedValue(new ApiError("forbidden", 403, { error: "no" }));
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.approve>>;
    await act(async () => {
      outcome = await result.current.approve();
    });

    expect(outcome).toEqual({ kind: "forbidden" });
    expect(useProposalsStore.getState().byCoordinator[COORDINATOR_ID]).toBeUndefined();
  });

  it("evicts the id from the store and returns a not_found outcome for a 404", async () => {
    seedProposal(COORDINATOR_ID, proposal());
    rejectProposalMock.mockRejectedValue(new ApiError("gone", 404, { error: "no" }));
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.reject>>;
    await act(async () => {
      outcome = await result.current.reject();
    });

    expect(outcome).toEqual({ kind: "not_found" });
    expect(
      useProposalsStore.getState().byCoordinator[COORDINATOR_ID]?.byId[PROPOSAL_ID],
    ).toBeUndefined();
  });
});

describe("useProposalDecision - network outcomes", () => {
  it("returns a network outcome for a thrown non-ApiError", async () => {
    approveProposalMock.mockRejectedValue(new TypeError("fetch failed"));
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.approve>>;
    await act(async () => {
      outcome = await result.current.approve();
    });

    expect(outcome).toEqual({ kind: "network" });
  });

  it("returns a network outcome for a 500", async () => {
    approveProposalMock.mockRejectedValue(new ApiError("boom", 500, { error: "boom" }));
    const { result } = renderHook(() =>
      useProposalDecision(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID),
    );

    let outcome!: Awaited<ReturnType<typeof result.current.approve>>;
    await act(async () => {
      outcome = await result.current.approve();
    });

    expect(outcome).toEqual({ kind: "network" });
    await waitFor(() => expect(result.current.busy).toBe(false));
  });
});

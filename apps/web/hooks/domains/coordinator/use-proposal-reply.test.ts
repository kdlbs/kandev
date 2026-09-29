import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Proposal } from "@/lib/api/domains/coordinator-api";

const replyMock = vi.fn();
const deliverMock = vi.fn();

vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return {
    ...actual,
    replyToProposal: (...args: unknown[]) => replyMock(...args),
    deliverProposalReply: (...args: unknown[]) => deliverMock(...args),
  };
});

import { REPLY_TEXT_MAX, replyTextLength, useProposalReply } from "./use-proposal-reply";
import { useProposalsStore } from "./use-proposals";

const WS = "w-1";
const COORD = "c-1";
const PID = "p-1";

function proposal(overrides: Partial<Proposal> = {}): Proposal {
  return {
    id: PID,
    coordinator_id: COORD,
    workspace_id: WS,
    status: "returned",
    spec: {
      title: "t",
      description: "d",
      rationale: "r",
      workflow_id: "wf",
      step_id: "s",
      repository_id: "repo",
      source_task_id: "t-1",
    },
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: "u-1",
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:05:00Z",
    reply_text: "narrower",
    reply_delivered_at: "2026-09-27T00:05:01Z",
    ...overrides,
  };
}

function errorOf(status: number, body: unknown): ApiError {
  return new ApiError("failed", status, body);
}

beforeEach(() => {
  useProposalsStore.setState({ byCoordinator: {} });
  replyMock.mockReset();
  deliverMock.mockReset();
});
afterEach(cleanup);

describe("replyTextLength", () => {
  it("counts Unicode code points, not UTF-16 units", () => {
    expect(replyTextLength("a😀b")).toBe(3);
    expect(REPLY_TEXT_MAX).toBe(2000);
  });
});

describe("useProposalReply", () => {
  it("locks while the request is in flight and stores the returned row", async () => {
    let resolve!: (p: Proposal) => void;
    replyMock.mockReturnValue(new Promise<Proposal>((r) => (resolve = r)));
    const { result } = renderHook(() => useProposalReply(WS, COORD, PID));
    let pending!: ReturnType<typeof result.current.reply>;
    act(() => {
      pending = result.current.reply("narrower");
    });
    expect(result.current.busy).toBe(true);
    resolve(proposal());
    let outcome!: Awaited<typeof pending>;
    await act(async () => {
      outcome = await pending;
    });
    expect(result.current.busy).toBe(false);
    expect(outcome.kind).toBe("decided");
    expect(replyMock).toHaveBeenCalledWith(WS, COORD, PID, "narrower");
    expect(useProposalsStore.getState().byCoordinator[COORD].byId[PID].status).toBe("returned");
  });

  it("returns a validation outcome naming the field and leaves the store untouched", async () => {
    replyMock.mockRejectedValue(errorOf(400, { error: "text is required", field: "text" }));
    const { result } = renderHook(() => useProposalReply(WS, COORD, PID));
    let outcome!: Awaited<ReturnType<typeof result.current.reply>>;
    await act(async () => {
      outcome = await result.current.reply("x");
    });
    expect(outcome).toEqual({ kind: "validation", message: "text is required", field: "text" });
    expect(useProposalsStore.getState().byCoordinator[COORD]?.byId[PID]).toBeUndefined();
  });

  it("maps a 5xx and a network failure on reply and deliver to network", async () => {
    replyMock.mockRejectedValue(errorOf(500, {}));
    deliverMock.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useProposalReply(WS, COORD, PID));
    await act(async () => {
      expect((await result.current.reply("x")).kind).toBe("network");
      expect((await result.current.redeliver()).kind).toBe("network");
    });
    expect(useProposalsStore.getState().byCoordinator[COORD]?.byId[PID]).toBeUndefined();
  });

  it("maps 403 to forbidden and 404 to not_found", async () => {
    replyMock.mockRejectedValueOnce(errorOf(403, {})).mockRejectedValueOnce(errorOf(404, {}));
    const { result } = renderHook(() => useProposalReply(WS, COORD, PID));
    await act(async () => {
      expect((await result.current.reply("x")).kind).toBe("forbidden");
      expect((await result.current.reply("x")).kind).toBe("not_found");
    });
  });

  it("stores the conflicting row of a 409 proposal_conflict", async () => {
    replyMock.mockRejectedValue(
      errorOf(409, { error_code: "proposal_conflict", proposal: proposal({ status: "approved" }) }),
    );
    const { result } = renderHook(() => useProposalReply(WS, COORD, PID));
    let outcome!: Awaited<ReturnType<typeof result.current.reply>>;
    await act(async () => {
      outcome = await result.current.reply("x");
    });
    expect(outcome.kind).toBe("conflict");
    expect(useProposalsStore.getState().byCoordinator[COORD].byId[PID].status).toBe("approved");
  });

  it("redeliver posts to the deliver route and stores the delivered row", async () => {
    deliverMock.mockResolvedValue(proposal());
    const { result } = renderHook(() => useProposalReply(WS, COORD, PID));
    await act(async () => {
      await result.current.redeliver();
    });
    expect(deliverMock).toHaveBeenCalledWith(WS, COORD, PID);
    expect(
      useProposalsStore.getState().byCoordinator[COORD].byId[PID].reply_delivered_at,
    ).toBeTruthy();
  });
});

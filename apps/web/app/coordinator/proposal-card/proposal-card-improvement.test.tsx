import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { ApiError } from "@/lib/api/client";
import type { ImprovementProposal } from "@/lib/api/domains/coordinator-api";
import type { PendingChangeStatus } from "@/lib/api/domains/coordinator-changes-api";
import type { UseProposalDecisionResult } from "@/hooks/domains/coordinator/use-proposal-decision";

const decisionState = vi.hoisted(() => ({
  current: {
    busy: false,
    approve: vi.fn().mockResolvedValue({ kind: "network" }),
    reject: vi.fn().mockResolvedValue({ kind: "network" }),
  } as UseProposalDecisionResult,
}));

vi.mock("@/hooks/domains/coordinator/use-proposal-decision", () => ({
  useProposalDecision: () => decisionState.current,
}));
vi.mock("@/hooks/domains/coordinator/use-proposal-edit-options", () => ({
  useProposalEditOptions: () => undefined,
}));
vi.mock("@/hooks/domains/settings/use-coordinator-phase3-effective", () => ({
  useCoordinatorPhase3Effective: () => true,
}));

const getRunMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/coordinator-autonomy-api", () => ({
  getRun: (...args: unknown[]) => getRunMock(...args),
}));
const fetchTaskMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
}));
vi.mock("../use-now-tick", () => ({
  useNowTick: () => new Date("2026-09-27T00:10:00Z").getTime(),
}));

import { ProposalCard } from "./proposal-card";
import { Phase2CardProvider } from "./phase2-context";

afterEach(() => {
  cleanup();
  getRunMock.mockReset();
  fetchTaskMock.mockReset();
  decisionState.current = {
    busy: false,
    approve: vi.fn().mockResolvedValue({ kind: "network" }),
    reject: vi.fn().mockResolvedValue({ kind: "network" }),
  };
});

function improvement(overrides: Record<string, unknown> = {}): ImprovementProposal {
  return {
    id: "p-1",
    coordinator_id: "c-1",
    workspace_id: "w-1",
    status: "pending",
    kind: "improvement",
    spec: {
      title: "Prefer small tasks",
      rationale: "Three runs stalled on large tasks.",
      context_before: "Be helpful.\nStay brief.",
      context_after: "Be helpful.\nPrefer small tasks.",
      evidence: [{ run_id: "r-1" }, { task_id: "t-9" }],
    },
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  } as ImprovementProposal;
}

function renderCard(proposal: ImprovementProposal, canManage = true) {
  return render(
    <ToastProvider>
      <Phase2CardProvider value={{ enabled: true, orders: undefined, offerReject: undefined }}>
        <ProposalCard
          proposal={proposal}
          variant="full"
          canManage={canManage}
          workspaceId="w-1"
          coordinatorId="c-1"
          workflowNameById={new Map()}
          stepNameByWorkflowStep={new Map()}
          coordinatorName="Coordinator"
        />
      </Phase2CardProvider>
    </ToastProvider>,
  );
}

const STARTED_AT = "2026-09-26T10:00:00Z";
const SHOW = "Show the change";
const APPROVE = "Approve as a reviewable change";

describe("improvement card content", () => {
  it("shows header, pill, title and rationale, with no policy line and no Edit", () => {
    getRunMock.mockResolvedValue({
      started_at: STARTED_AT,
      outcome: null,
      cost_subcents: null,
    });
    fetchTaskMock.mockResolvedValue({ identifier: "KAN-9", title: "Big task" });
    renderCard(improvement());
    expect(screen.getByText("Improvement")).not.toBeNull();
    expect(screen.getByText("Changes coordinator context")).not.toBeNull();
    expect(screen.getByText("Prefer small tasks")).not.toBeNull();
    expect(screen.getByText("Three runs stalled on large tasks.")).not.toBeNull();
    expect(screen.queryByText(/^Policy:/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("keeps Approve disabled with a hint until the diff is shown", () => {
    getRunMock.mockResolvedValue({ started_at: STARTED_AT, outcome: null });
    fetchTaskMock.mockResolvedValue({ identifier: "K", title: "T" });
    renderCard(improvement());
    const approve = screen.getByRole("button", { name: APPROVE }) as HTMLButtonElement;
    expect(approve.disabled).toBe(true);
    expect(screen.getByText("Show the change first")).not.toBeNull();
    expect(screen.queryByTestId("context-diff")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: SHOW }));
    expect(screen.getByTestId("context-diff")).not.toBeNull();
    expect((screen.getByRole("button", { name: APPROVE }) as HTMLButtonElement).disabled).toBe(
      false,
    );
    expect(screen.queryByText("Show the change first")).toBeNull();
  });

  it("maps a run 404 to expired and another failure to unavailable, per entry", async () => {
    getRunMock
      .mockRejectedValueOnce(new ApiError("gone", 404, null))
      .mockRejectedValueOnce(new ApiError("boom", 500, null));
    fetchTaskMock.mockRejectedValue(new Error("nope"));
    renderCard(
      improvement({
        spec: {
          title: "T",
          rationale: "R",
          context_before: "a",
          context_after: "b",
          evidence: [{ run_id: "r-1" }, { run_id: "r-2" }, { task_id: "t-9" }],
        },
      }),
    );
    await waitFor(() => expect(screen.getByText("Run record expired")).not.toBeNull());
    expect(screen.getByText("Run unavailable")).not.toBeNull();
    await waitFor(() => expect(screen.getByText("Task no longer available")).not.toBeNull());
  });

  it("reads an in-progress run and omits a null cost", async () => {
    getRunMock.mockResolvedValue({
      started_at: STARTED_AT,
      outcome: null,
      cost_subcents: null,
    });
    fetchTaskMock.mockResolvedValue({ identifier: "KAN-9", title: "Big task" });
    renderCard(improvement());
    await waitFor(() => expect(screen.getByTestId("improvement-run-line")).not.toBeNull());
    expect(screen.getByTestId("improvement-run-line").textContent).toContain("In progress");
    await waitFor(() => expect(screen.getByRole("link", { name: /KAN-9/ })).not.toBeNull());
  });
});

describe("improvement card decisions", () => {
  it("approves through the shared decision and offers Reject and Reply", () => {
    getRunMock.mockResolvedValue({ started_at: STARTED_AT, outcome: "ok" });
    fetchTaskMock.mockResolvedValue({ identifier: "K", title: "T" });
    renderCard(improvement());
    fireEvent.click(screen.getByRole("button", { name: SHOW }));
    fireEvent.click(screen.getByRole("button", { name: APPROVE }));
    expect(decisionState.current.approve).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Reject" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Reply with a condition" })).not.toBeNull();
  });

  it("readers see the evidence and diff control but no decision controls", () => {
    getRunMock.mockResolvedValue({ started_at: STARTED_AT, outcome: "ok" });
    fetchTaskMock.mockResolvedValue({ identifier: "K", title: "T" });
    renderCard(improvement(), false);
    expect(screen.getByRole("button", { name: SHOW })).not.toBeNull();
    expect(screen.queryByRole("button", { name: APPROVE })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reject" })).toBeNull();
  });
});

describe("improvement card states", () => {
  it("failed card reads one fixed sentence and keeps Approve gated with Reject, no Reply", () => {
    getRunMock.mockResolvedValue({ started_at: STARTED_AT, outcome: "ok" });
    fetchTaskMock.mockResolvedValue({ identifier: "K", title: "T" });
    renderCard(improvement({ status: "failed", error: "outcome_unknown: raw text" }));
    expect(
      screen.getAllByText("Approving did not finish. Nothing was applied. You can approve again.")
        .length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText(/raw text/)).toBeNull();
    expect((screen.getByRole("button", { name: APPROVE }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(screen.queryByRole("button", { name: "Reply with a condition" })).toBeNull();
  });

  it("stale approving offers Retry only, gated by the diff", () => {
    getRunMock.mockResolvedValue({ started_at: STARTED_AT, outcome: "ok" });
    fetchTaskMock.mockResolvedValue({ identifier: "K", title: "T" });
    renderCard(improvement({ status: "approving", claimed_at: "2026-09-26T00:00:00Z" }));
    expect(screen.queryByRole("button", { name: "Reject" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reply with a condition" })).toBeNull();
    const retry = screen.getByRole("button", { name: "Retry" }) as HTMLButtonElement;
    expect(retry.disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: SHOW }));
    expect((screen.getByRole("button", { name: "Retry" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it.each<[PendingChangeStatus | null, string, boolean]>([
    [null, "Approved as a reviewable change. Nothing was applied.", true],
    ["pending", "Approved as a reviewable change. Nothing was applied.", true],
    ["applied", "Approved as a reviewable change. Applied to the context.", false],
    ["discarded", "Approved as a reviewable change. Discarded, nothing was applied.", false],
  ])("approved line for change status %s", (changeStatus, line, hasLink) => {
    getRunMock.mockResolvedValue({ started_at: STARTED_AT, outcome: "ok" });
    fetchTaskMock.mockResolvedValue({ identifier: "K", title: "T" });
    renderCard(improvement({ status: "approved", change_status: changeStatus }));
    expect(screen.getAllByText(line).length).toBeGreaterThan(0);
    const link = screen.queryByRole("link", { name: "Open settings" });
    expect(link !== null).toBe(hasLink);
    if (link) {
      expect(link.getAttribute("href")).toBe(
        "/settings/workspaces/w-1/coordinators/c-1?section=autonomy",
      );
    }
    expect(screen.queryByRole("button", { name: APPROVE })).toBeNull();
  });
});

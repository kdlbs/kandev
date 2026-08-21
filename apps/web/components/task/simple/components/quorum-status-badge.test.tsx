import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { QuorumGuardEntry, QuorumResponseDTO } from "@/lib/state/slices/office/quorum-types";
import type { Task } from "@/app/office/tasks/[id]/types";

const getTaskQuorumMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-extended-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-extended-api")>(
    "@/lib/api/domains/office-extended-api",
  );
  return {
    ...actual,
    getTaskQuorum: getTaskQuorumMock,
  };
});

import { QuorumStatusBadge, aggregateQuorum } from "./quorum-status-badge";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const TS = "2026-05-01T00:00:00Z";

const baseTask: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "in_review",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "user",
  createdAt: TS,
  updatedAt: TS,
};

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </StateProvider>
  );
}

const awaitingEntry: QuorumGuardEntry = {
  target_step_id: "step-2",
  role: "approver",
  threshold: "n_approve:2",
  required_count: 2,
  received_count: 1,
  satisfied: false,
  reason: "threshold_not_met",
};

const stuckEntry: QuorumGuardEntry = {
  target_step_id: "step-2",
  role: "approver",
  threshold: "n_approve:foo",
  required_count: 0,
  received_count: 0,
  satisfied: false,
  reason: "threshold_unrecognized",
};

describe("aggregateQuorum", () => {
  it("aggregates to clear for an empty guard list", () => {
    expect(aggregateQuorum([], false)).toEqual({ state: "clear" });
  });

  it("aggregates to awaiting when an entry is unsatisfied on threshold_not_met", () => {
    expect(aggregateQuorum([awaitingEntry], false)).toEqual({
      state: "awaiting",
      entry: awaitingEntry,
    });
  });

  it("aggregates to stuck when an entry reports a non-threshold_not_met reason", () => {
    expect(aggregateQuorum([awaitingEntry, stuckEntry], false)).toEqual({
      state: "stuck",
      entry: stuckEntry,
    });
  });

  it("aggregates to stuck with no selected entry when only reevaluation_blocked fires", () => {
    expect(aggregateQuorum([], true)).toEqual({ state: "stuck", entry: undefined });
  });

  it("aggregates to stuck when reevaluation_blocked holds even with all entries satisfied", () => {
    const satisfied: QuorumGuardEntry = { ...awaitingEntry, satisfied: true, reason: undefined };
    expect(aggregateQuorum([satisfied], true)).toEqual({ state: "stuck", entry: undefined });
  });

  it("aggregates to clear when every entry is satisfied and not blocked", () => {
    const satisfied: QuorumGuardEntry = { ...awaitingEntry, satisfied: true, reason: undefined };
    expect(aggregateQuorum([satisfied], false)).toEqual({ state: "clear" });
  });
});

const BADGE_TEST_ID = "quorum-status-badge";

function seedQuorum(quorum: QuorumResponseDTO) {
  getTaskQuorumMock.mockResolvedValue(quorum);
}

describe("QuorumStatusBadge", () => {
  it("fetches when a guarded workflow step has an in-progress legacy status", async () => {
    seedQuorum({ guards: [awaitingEntry], reevaluation_blocked: false });
    render(
      <Wrapper>
        <QuorumStatusBadge task={{ ...baseTask, status: "todo" }} />
      </Wrapper>,
    );
    await vi.waitFor(() =>
      expect(getTaskQuorumMock).toHaveBeenCalledWith(baseTask.id, baseTask.workspaceId),
    );
    expect(await screen.findByTestId(BADGE_TEST_ID)).toBeTruthy();
  });

  it("hides when the guard list aggregates to clear", async () => {
    seedQuorum({ guards: [], reevaluation_blocked: false });
    render(
      <Wrapper>
        <QuorumStatusBadge task={baseTask} />
      </Wrapper>,
    );
    await vi.waitFor(() =>
      expect(getTaskQuorumMock).toHaveBeenCalledWith(baseTask.id, baseTask.workspaceId),
    );
    expect(screen.queryByTestId(BADGE_TEST_ID)).toBeNull();
  });

  it("renders the awaiting state with received/required counts", async () => {
    seedQuorum({ guards: [awaitingEntry], reevaluation_blocked: false });
    render(
      <Wrapper>
        <QuorumStatusBadge task={baseTask} />
      </Wrapper>,
    );
    const badge = await screen.findByTestId(BADGE_TEST_ID);
    expect(badge.textContent).toContain("1");
    expect(badge.textContent).toContain("2");
    expect(badge.getAttribute("data-variant")).toBe("outline");
  });

  it("renders the stuck state naming the selected entry's reason", async () => {
    seedQuorum({ guards: [stuckEntry], reevaluation_blocked: false });
    render(
      <Wrapper>
        <QuorumStatusBadge task={baseTask} />
      </Wrapper>,
    );
    const badge = await screen.findByTestId(BADGE_TEST_ID);
    expect(badge.getAttribute("data-variant")).toBe("destructive");
    expect(screen.getByTestId("quorum-status-reason").textContent).not.toBe("");
  });

  it("renders stuck when only reevaluation_blocked fires with a satisfied entry", async () => {
    const satisfied: QuorumGuardEntry = { ...awaitingEntry, satisfied: true, reason: undefined };
    seedQuorum({ guards: [satisfied], reevaluation_blocked: true });
    render(
      <Wrapper>
        <QuorumStatusBadge task={baseTask} />
      </Wrapper>,
    );
    const badge = await screen.findByTestId(BADGE_TEST_ID);
    expect(badge.getAttribute("data-variant")).toBe("destructive");
  });
});

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MergeProposal } from "@/lib/api/domains/coordinator-api";
import type { MergeDetail } from "@/lib/api/domains/coordinator-merge-api";

const HEAD_SHA = "0123456789";
const APPROVE_TESTID = "merge-approve";
const toast = vi.fn();
const approve = vi.fn();
const reject = vi.fn();
const checkNow = vi.fn();
const release = vi.fn();
let detail: MergeDetail | null = null;
let busy = false;

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
vi.mock("@/hooks/domains/coordinator/use-proposal-decision", () => ({
  useProposalDecision: () => ({ busy, approve, reject }),
}));
vi.mock("@/hooks/domains/coordinator/use-merge-detail", () => ({
  useMergeDetail: () => ({
    detail,
    status: "ready",
    busy: false,
    actionError: null,
    reload: vi.fn(),
    checkNow,
    release,
  }),
}));

import { MergeProposalCard } from "./merge-card";

beforeEach(() => {
  vi.clearAllMocks();
  detail = null;
  busy = false;
});
afterEach(cleanup);

function proposal(status: MergeProposal["status"]): MergeProposal {
  return {
    id: "p1",
    coordinator_id: "c1",
    workspace_id: "w1",
    status,
    kind: "merge",
    reject_reason: null,
    spec: {
      task_id: "t1",
      repository_id: "r1",
      change_key: "github|github.com|r1|12",
      head_sha: HEAD_SHA,
      method: "squash",
    },
    final_spec: null,
    updated_at: "2026-05-01T10:00:00Z",
  } as unknown as MergeProposal;
}

function mount(status: MergeProposal["status"], canManage = true) {
  return render(
    <MergeProposalCard
      proposal={proposal(status)}
      variant="full"
      canManage={canManage}
      workspaceId="w1"
      coordinatorId="c1"
      coordinatorName="Sheph"
    />,
  );
}

const live = {
  state: "open",
  head_sha: HEAD_SHA,
  draft: "met",
  pipeline: "green",
  mergeable: "met",
  conflict: "clear",
  unresolved_threads: 0,
  approvals: "met",
} as const;

describe("MergeProposalCard", () => {
  it("shows the owner, the conditions and the no-undo statement but never Undo", () => {
    detail = {
      change_key: "github|github.com|r1|12",
      repository_id: "r1",
      head_sha: HEAD_SHA,
      method: "squash",
      automatic: false,
      manager_only: false,
      live,
    };
    mount("pending");
    expect(screen.getByTestId("merge-card-title").textContent).toContain("12");
    expect(screen.getByText(/Sheph/)).toBeTruthy();
    expect(screen.getByTestId("merge-cannot-undo").textContent).toBe(
      "A merge cannot be undone here.",
    );
    expect(screen.queryByText(/undo/i, { selector: "button" })).toBeNull();
    expect(screen.getByTestId(APPROVE_TESTID)).toBeTruthy();
  });

  it("shows the manager-only note", () => {
    detail = {
      change_key: "k",
      repository_id: "r1",
      head_sha: HEAD_SHA,
      method: "squash",
      automatic: false,
      manager_only: true,
      note: "Daily limit reached",
    };
    mount("pending");
    expect(screen.getByTestId("merge-card-note").textContent).toBe("Daily limit reached");
  });

  it("offers Check now for an uncertain merge and no Approve", () => {
    detail = {
      change_key: "k",
      repository_id: "r1",
      head_sha: HEAD_SHA,
      method: "squash",
      automatic: true,
      manager_only: false,
      dispatch: {
        outcome: "uncertain",
        queued: false,
        dispatched_at: "2026-05-01T10:00:00Z",
        can_check: true,
        can_release: false,
      },
    };
    mount("uncertain");
    expect(screen.queryByTestId(APPROVE_TESTID)).toBeNull();
    expect(screen.queryByTestId("merge-release")).toBeNull();
    fireEvent.click(screen.getByTestId("merge-check-now"));
    expect(checkNow).toHaveBeenCalledTimes(1);
  });

  it("offers Release only once the backend allows it", () => {
    detail = {
      change_key: "k",
      repository_id: "r1",
      head_sha: HEAD_SHA,
      method: "squash",
      automatic: true,
      manager_only: false,
      dispatch: {
        outcome: "uncertain",
        queued: false,
        dispatched_at: "2026-05-01T10:00:00Z",
        can_check: true,
        can_release: true,
      },
    };
    mount("uncertain");
    fireEvent.click(screen.getByTestId("merge-release"));
    expect(release).toHaveBeenCalledTimes(1);
  });

  it("sends one approval for a double click", async () => {
    let resolve!: (v: unknown) => void;
    approve.mockReturnValue(new Promise((r) => (resolve = r)));
    mount("pending");
    const button = screen.getByTestId(APPROVE_TESTID);
    fireEvent.click(button);
    fireEvent.click(button);
    expect(approve).toHaveBeenCalledTimes(1);
    await act(async () => resolve({ kind: "decided", proposal: proposal("approved") }));
    expect(toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "success" }));
  });

  it("hides the decision buttons without manage rights", () => {
    mount("pending", false);
    expect(screen.queryByTestId(APPROVE_TESTID)).toBeNull();
    expect(screen.queryByTestId("merge-reject")).toBeNull();
  });
});

import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type {
  KindProposal,
  StandingOrder,
  StoredProposal,
} from "@/lib/api/domains/coordinator-api";
import type { UseProposalDecisionResult } from "@/hooks/domains/coordinator/use-proposal-decision";
import type { AttentionTask } from "@/lib/coordinator/attention";

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

const fetchTaskMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
}));

vi.mock("../use-now-tick", () => ({
  useNowTick: () => new Date("2026-09-27T00:10:00Z").getTime(),
}));

import { ProposalCard, type ProposalCardProps } from "./proposal-card";
import { Phase2CardProvider } from "./phase2-context";

afterEach(() => {
  cleanup();
  fetchTaskMock.mockReset();
  decisionState.current = {
    busy: false,
    approve: vi.fn().mockResolvedValue({ kind: "network" }),
    reject: vi.fn().mockResolvedValue({ kind: "network" }),
  };
});

const TASKS = new Map<string, AttentionTask>([
  ["t-1", { id: "t-1", title: "Fix login", identifier: "KAN-418" }],
]);
const STEP_NAMES = new Map([
  ["wf-1:a", "Build"],
  ["wf-1:b", "Review"],
]);

function row(kind: KindProposal["kind"], overrides: Record<string, unknown> = {}): KindProposal {
  const specs = {
    resume: { task_id: "t-1", rationale: "Its agent stopped." },
    message: { task_id: "t-1", text: "Please rebase.", rationale: "Behind main." },
    move: {
      task_id: "t-1",
      workflow_id: "wf-1",
      from_step_id: "a",
      to_step_id: "b",
      rationale: "Ready.",
    },
  };
  return {
    id: "p-1",
    coordinator_id: "c-1",
    workspace_id: "w-1",
    status: "pending",
    kind,
    spec: specs[kind],
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  } as KindProposal;
}

function order(id: string, number: number | null, text: string): StandingOrder {
  return {
    id,
    number,
    text,
    created_at: "2026-09-12T00:00:00Z",
    created_by: "u",
    retired_at: number === null ? "2026-09-20T00:00:00Z" : null,
    last_applied_at: null,
  };
}

function renderCard(
  proposal: StoredProposal,
  overrides: Partial<ProposalCardProps> = {},
  context: Parameters<typeof Phase2CardProvider>[0]["value"] = {
    enabled: true,
    orders: undefined,
    offerReject: undefined,
  },
) {
  const props: ProposalCardProps = {
    proposal,
    variant: "full",
    canManage: true,
    workspaceId: "w-1",
    coordinatorId: "c-1",
    workflowNameById: new Map(),
    stepNameByWorkflowStep: STEP_NAMES,
    openTasksById: TASKS,
    coordinatorName: "Coordinator",
    ...overrides,
  };
  return render(
    <ToastProvider>
      <Phase2CardProvider value={context}>
        <ProposalCard {...props} />
      </Phase2CardProvider>
    </ToastProvider>,
  );
}

const ORDER_TEXT = "Keep it small";
const OFFER_ACTION = "Make it a standing order";

describe("kind cards: title, target link and copy", () => {
  it("resume: title links to the task, with rationale and its policy line", () => {
    renderCard(row("resume"));
    const link = screen.getByRole("link", { name: "KAN-418" });
    expect(link.getAttribute("href")).toContain("t-1");
    expect(screen.getByText(/^Resume/)).not.toBeNull();
    expect(screen.getByText("Its agent stopped.")).not.toBeNull();
    expect(screen.getByText("Policy: Resume a task requires approval")).not.toBeNull();
  });

  it("message: quotes the text and offers Edit", () => {
    renderCard(row("message"));
    expect(screen.getByText("Please rebase.")).not.toBeNull();
    expect(screen.getByText("Policy: Message a task requires approval")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Edit" })).not.toBeNull();
  });

  it("move: names both steps and shows the starts-agent line only when stored true", () => {
    const { unmount } = renderCard(row("move", { starts_agent: true }));
    expect(screen.getByText(/from Build to Review/)).not.toBeNull();
    expect(screen.getByText("Approving this starts an agent")).not.toBeNull();
    expect(screen.getByText("Policy: Move a task requires approval")).not.toBeNull();
    unmount();
    renderCard(row("move", { starts_agent: false }));
    expect(screen.queryByText("Approving this starts an agent")).toBeNull();
  });

  it("resume and move have no Edit button", () => {
    const { unmount } = renderCard(row("resume"));
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    unmount();
    renderCard(row("move"));
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("an unknown target task shows its id with no link, and Approve and Reject stay", () => {
    renderCard(row("resume", { spec: { task_id: "t-9", rationale: "r" } }));
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText("t-9")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Approve" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Reject" })).not.toBeNull();
  });

  it("an unresolved step shows its raw id", () => {
    renderCard(
      row("move", {
        spec: {
          task_id: "t-1",
          workflow_id: "wf-1",
          from_step_id: "gone",
          to_step_id: "b",
          rationale: "r",
        },
      }),
    );
    expect(screen.getByText(/from gone to Review/)).not.toBeNull();
  });
});

describe("kind cards: Shaped by", () => {
  it("labels active and retired orders in stored order and shows nothing for a missing id", () => {
    renderCard(
      row("resume", { standing_order_ids: ["o2", "o1", "o-missing"] }),
      {},
      {
        enabled: true,
        orders: [order("o1", null, "Old rule"), order("o2", 2, ORDER_TEXT)],
        offerReject: undefined,
      },
    );
    const labels = screen.getAllByRole("button", { name: /Shaped by/ });
    expect(labels.map((l) => l.textContent)).toEqual([
      "Shaped by: Standing order 2",
      "Shaped by: a retired standing order",
    ]);
  });

  it("reveals the order text on hover, keyboard activation and tap", async () => {
    renderCard(
      row("resume", { standing_order_ids: ["o2"] }),
      {},
      {
        enabled: true,
        orders: [order("o2", 2, ORDER_TEXT)],
        offerReject: undefined,
      },
    );
    const label = screen.getByRole("button", { name: /Shaped by/ });
    expect(screen.queryByText(ORDER_TEXT)).toBeNull();
    fireEvent.mouseEnter(label);
    expect(await screen.findByText(ORDER_TEXT)).not.toBeNull();
    fireEvent.mouseLeave(label);
    await waitFor(() => expect(screen.queryByText(ORDER_TEXT)).toBeNull());
    label.focus();
    fireEvent.keyDown(label, { key: "Enter" });
    fireEvent.click(label);
    expect(await screen.findByText(ORDER_TEXT)).not.toBeNull();
    fireEvent.click(label);
    await waitFor(() => expect(screen.queryByText(ORDER_TEXT)).toBeNull());
    fireEvent.click(label);
    expect(await screen.findByText(ORDER_TEXT)).not.toBeNull();
  });

  it("shows no label while the orders are unloaded, and the compact card omits Policy and Shaped by", () => {
    renderCard(row("resume", { standing_order_ids: ["o1"] }));
    expect(screen.queryByText(/Shaped by/)).toBeNull();
    cleanup();
    renderCard(
      row("resume", { standing_order_ids: ["o1"], starts_agent: true }),
      { variant: "compact" },
      { enabled: true, orders: [order("o1", 1, "x")], offerReject: undefined },
    );
    expect(screen.queryByText(/Shaped by/)).toBeNull();
    expect(screen.queryByText(/^Policy:/)).toBeNull();
  });
});

describe("kind cards: message Edit", () => {
  it("approves with the trimmed text only, and blocks empty and over-long text", () => {
    renderCard(row("message"));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    const area = screen.getByLabelText("Message text");
    const submit = screen.getByRole("button", { name: "Approve with edits" });

    fireEvent.change(area, { target: { value: "   " } });
    expect((submit as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(area, { target: { value: "x".repeat(4001) } });
    expect((submit as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(area, { target: { value: "  new text  " } });
    expect((submit as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(submit);
    expect(decisionState.current.approve).toHaveBeenCalledWith({ text: "new text" });
  });

  it("counts emoji as one code point each", () => {
    renderCard(row("message"));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(screen.getByLabelText("Message text"), {
      target: { value: "😀".repeat(4000) },
    });
    expect(
      (screen.getByRole("button", { name: "Approve with edits" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });
});

describe("kind cards: failure, approval and policy_denied", () => {
  it("shows the per-code failure copy and keeps Approve and Reject", () => {
    renderCard(row("move", { status: "failed", error: "step_full" }));
    expect(
      screen.getAllByText(
        "The destination step is at its work-in-progress limit. The task was not moved.",
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Approve" })).not.toBeNull();
  });

  it("names the target task in the approved status line", () => {
    renderCard(row("resume", { status: "approved" }));
    expect(screen.getAllByText("Approved: KAN-418").length).toBeGreaterThan(0);
  });

  it("hides Approve and Edit and keeps Reject after a policy_denied decision", async () => {
    decisionState.current = {
      busy: false,
      approve: vi.fn().mockResolvedValue({ kind: "policy_denied" }),
      reject: vi.fn(),
    };
    renderCard(row("message"));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    });
    expect(screen.getByText("Its May do settings no longer allow this")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.getByRole("button", { name: "Reject" })).not.toBeNull();
  });

  it("toasts the kind-specific approval", async () => {
    decisionState.current = {
      busy: false,
      approve: vi.fn().mockResolvedValue({
        kind: "decided",
        proposal: row("move", { status: "approved", outcome: { queued: true } }),
      }),
      reject: vi.fn(),
    };
    renderCard(row("move"));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    });
    expect(
      await screen.findByText("Approved. KAN-418 is queued behind the limit of Review."),
    ).not.toBeNull();
  });
});

describe("kind cards: reject offer", () => {
  it("offers the reason as a standing order and calls the host on the action", async () => {
    const offerReject = vi.fn();
    decisionState.current = {
      busy: false,
      approve: vi.fn(),
      reject: vi.fn().mockResolvedValue({
        kind: "decided",
        proposal: row("resume", { status: "rejected" }),
      }),
    };
    renderCard(row("resume"), {}, { enabled: true, orders: undefined, offerReject });
    fireEvent.click(screen.getByRole("button", { name: "Reject" }));
    fireEvent.change(screen.getByLabelText("Reason (optional)"), {
      target: { value: "  Too noisy  " },
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Confirm reject" }));
    });
    fireEvent.click(await screen.findByRole("button", { name: OFFER_ACTION }));
    expect(offerReject).toHaveBeenCalledWith({ proposalId: "p-1", reason: "Too noisy" });
  });

  it("keeps the offer for 10 seconds and then drops it", async () => {
    vi.useFakeTimers();
    try {
      decisionState.current = {
        busy: false,
        approve: vi.fn(),
        reject: vi.fn().mockResolvedValue({
          kind: "decided",
          proposal: row("resume", { status: "rejected" }),
        }),
      };
      renderCard(row("resume"), {}, { enabled: true, orders: undefined, offerReject: vi.fn() });
      fireEvent.click(screen.getByRole("button", { name: "Reject" }));
      fireEvent.change(screen.getByLabelText("Reason (optional)"), {
        target: { value: "Too noisy" },
      });
      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: "Confirm reject" }));
      });
      expect(screen.getByRole("button", { name: OFFER_ACTION })).not.toBeNull();
      act(() => {
        vi.advanceTimersByTime(9_000);
      });
      expect(screen.getByRole("button", { name: OFFER_ACTION })).not.toBeNull();
      act(() => {
        vi.advanceTimersByTime(1_500);
      });
      expect(screen.queryByRole("button", { name: OFFER_ACTION })).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows the plain toast for a blank reason", async () => {
    decisionState.current = {
      busy: false,
      approve: vi.fn(),
      reject: vi.fn().mockResolvedValue({
        kind: "decided",
        proposal: row("resume", { status: "rejected" }),
      }),
    };
    renderCard(row("resume"), {}, { enabled: true, orders: undefined, offerReject: vi.fn() });
    fireEvent.click(screen.getByRole("button", { name: "Reject" }));
    fireEvent.change(screen.getByLabelText("Reason (optional)"), { target: { value: "   " } });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Confirm reject" }));
    });
    expect(screen.queryByRole("button", { name: OFFER_ACTION })).toBeNull();
  });
});

describe("kind cards: compact card reads its own task", () => {
  it("fetches the target task when no task map is given", async () => {
    fetchTaskMock.mockResolvedValue({ id: "t-1", identifier: "KAN-418" });
    renderCard(row("resume"), { variant: "compact", openTasksById: undefined });
    expect(await screen.findByRole("link", { name: "KAN-418" })).not.toBeNull();
    expect(fetchTaskMock).toHaveBeenCalledWith("t-1");
  });
});

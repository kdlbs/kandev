import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { ReactElement } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";
import { useProposalsStore } from "@/hooks/domains/coordinator/use-proposals";
import type { Proposal } from "@/lib/api/domains/coordinator-api";
import type {
  AttentionTask,
  NeedsYouErrorItem,
  NeedsYouProposalItem,
  NeedsYouQuestionItem,
  NeedsYouStallItem,
} from "@/lib/coordinator/attention";
import { NeedsYouItemCard, needsYouItemHeadingId } from "./needs-you-item-card";

const TIMESTAMP = "2026-09-27T00:00:00Z";

afterEach(cleanup);

const WORKSPACE_ID = "ws-1";
const COORDINATOR_ID = "co-1";

function task(id: string, overrides: Partial<AttentionTask> = {}): AttentionTask {
  return { id, title: `Task ${id}`, ...overrides };
}

function questionItem(overrides: Partial<NeedsYouQuestionItem> = {}): NeedsYouQuestionItem {
  return {
    kind: "question",
    id: "t-1",
    task: task("t-1", { identifier: "KAN-1" }),
    pendingAction: "clarification",
    referenceTimeMs: 0,
    ageMs: 5 * 60_000,
    ...overrides,
  };
}

function stallItem(overrides: Partial<NeedsYouStallItem> = {}): NeedsYouStallItem {
  return {
    kind: "stall",
    id: "t-2",
    task: task("t-2", { identifier: "KAN-2" }),
    stall: {
      task_id: "t-2",
      stalled_for_ms: 4 * 3_600_000 + 12 * 60_000,
      last_event_at: TIMESTAMP,
      detected_at: "2026-09-27T00:01:00Z",
    },
    referenceTimeMs: 0,
    ageMs: 4 * 3_600_000 + 12 * 60_000,
    ...overrides,
  };
}

function errorItem(overrides: Partial<NeedsYouErrorItem> = {}): NeedsYouErrorItem {
  return {
    kind: "error",
    id: "t-3",
    task: task("t-3", { identifier: "KAN-3" }),
    activeError: { preview: "boom" },
    taskError: null,
    referenceTimeMs: 0,
    ageMs: 60_000,
    ...overrides,
  };
}

function proposalItem(overrides: Partial<NeedsYouProposalItem> = {}): NeedsYouProposalItem {
  return {
    kind: "proposal",
    id: "p-1",
    referenceTimeMs: 0,
    ageMs: 60_000,
    proposal: {
      id: "p-1",
      status: "pending",
      task_id: null,
      created_at: TIMESTAMP,
      spec: {
        title: "Add tests",
        description: "Coverage is thin here",
        rationale: "Because coverage is thin",
        workflow_id: "wf-1",
        step_id: "step-1",
        repository_id: "repo-1",
        source_task_id: "t-4",
      },
    },
    ...overrides,
  };
}

/** The full `Proposal` row `useProposalRow` reads, matching `proposalItem()`'s narrow shape. */
function proposalRow(overrides: Partial<Proposal> = {}): Proposal {
  return {
    id: "p-1",
    coordinator_id: COORDINATOR_ID,
    workspace_id: WORKSPACE_ID,
    status: "pending",
    spec: {
      title: "Add tests",
      description: "Coverage is thin here",
      rationale: "Because coverage is thin",
      workflow_id: "wf-1",
      step_id: "step-1",
      repository_id: "repo-1",
      source_task_id: "t-4",
    },
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

const NO_OP_MAPS = {
  workspaceId: WORKSPACE_ID,
  stepNameByTaskId: new Map<string, string>(),
  workflowNameById: new Map<string, string>(),
  stepNameByWorkflowStep: new Map<string, string>(),
  openTasksById: new Map<string, AttentionTask>(),
  coordinatorId: COORDINATOR_ID,
  canManage: true,
};

const SHOW_THE_EVIDENCE = "Show the evidence";
const ASK_ABOUT_THIS = "Ask about this";

function renderCard(ui: ReactElement) {
  return render(
    <ToastProvider>
      <TooltipProvider>{ui}</TooltipProvider>
    </ToastProvider>,
  );
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

afterEach(() => {
  useCopilotStore.setState({ entries: {} });
  useProposalsStore.setState({ byCoordinator: {} });
});

describe("NeedsYouItemCard - head, severity and age", () => {
  it("shows the task identifier, step, decide-now severity and age for a question item", () => {
    renderCard(
      <NeedsYouItemCard
        item={questionItem()}
        {...NO_OP_MAPS}
        stepNameByTaskId={new Map([["t-1", "Build"]])}
        coordinatorName="Planner"
      />,
    );

    expect(screen.getByText("KAN-1")).not.toBeNull();
    expect(screen.getByText("Build")).not.toBeNull();
    expect(screen.getByText("Decide now")).not.toBeNull();
    expect(screen.getByText("5m")).not.toBeNull();
  });

  it("shows Review severity for a proposal", () => {
    renderCard(
      <NeedsYouItemCard item={proposalItem()} {...NO_OP_MAPS} coordinatorName="Planner" />,
    );
    expect(screen.getByText("Review")).not.toBeNull();
  });

  it("falls back to the task title when it has no identifier", () => {
    renderCard(
      <NeedsYouItemCard
        item={questionItem({ task: task("t-1") })}
        {...NO_OP_MAPS}
        coordinatorName="Planner"
      />,
    );
    expect(screen.getByText("Task t-1")).not.toBeNull();
  });

  it("stripes the card by severity, so the list is scannable without the badges", () => {
    const item = errorItem();
    render(<NeedsYouItemCard item={item} {...NO_OP_MAPS} coordinatorName="Planner" />);
    expect(screen.getByTestId(`needs-you-item-${item.id}`).className).toContain(
      "border-l-destructive",
    );
  });

  it("stripes a proposal as review rather than decide now", () => {
    const item = proposalItem();
    render(<NeedsYouItemCard item={item} {...NO_OP_MAPS} coordinatorName="Planner" />);
    expect(screen.getByTestId(`needs-you-item-${item.id}`).className).toContain("border-l-primary");
  });

  it("pushes the age to the end of the head row", () => {
    const item = errorItem();
    render(<NeedsYouItemCard item={item} {...NO_OP_MAPS} coordinatorName="Planner" />);
    expect(screen.getByText("1m").className).toContain("ml-auto");
  });

  it("gives the item a focusable heading with a stable id", () => {
    renderCard(
      <NeedsYouItemCard item={questionItem()} {...NO_OP_MAPS} coordinatorName="Planner" />,
    );
    const heading = document.getElementById(needsYouItemHeadingId("t-1"));
    expect(heading).not.toBeNull();
    expect(heading?.getAttribute("tabindex")).toBe("-1");
  });
});

describe("NeedsYouItemCard - why/clears text by kind", () => {
  it("shows the fixed question texts", () => {
    renderCard(
      <NeedsYouItemCard item={questionItem()} {...NO_OP_MAPS} coordinatorName="Planner" />,
    );
    expect(screen.getByText("The agent is waiting for your answer")).not.toBeNull();
    expect(screen.getByText("Your answer, on the task")).not.toBeNull();
  });

  it("shows the stalled-for duration in the stall why text", () => {
    renderCard(<NeedsYouItemCard item={stallItem()} {...NO_OP_MAPS} coordinatorName="Planner" />);
    expect(screen.getByText("No activity for 4h 12m, and no agent is running")).not.toBeNull();
    expect(screen.getByText("Resuming or restarting the task")).not.toBeNull();
  });

  it("shows the active error's preview", () => {
    renderCard(<NeedsYouItemCard item={errorItem()} {...NO_OP_MAPS} coordinatorName="Planner" />);
    expect(screen.getByText("The agent reported an error: boom")).not.toBeNull();
  });

  it("falls back to 'the task failed' with no preview when there is no active error", () => {
    renderCard(
      <NeedsYouItemCard
        item={errorItem({ activeError: null, taskError: { preview: "ignored" } })}
        {...NO_OP_MAPS}
        coordinatorName="Planner"
      />,
    );
    expect(screen.getByText("The task failed")).not.toBeNull();
    expect(screen.queryByText(/ignored/)).toBeNull();
  });

  it("shows the coordinator's rationale and the approve/edit/reject clears text for a proposal", () => {
    renderCard(
      <NeedsYouItemCard item={proposalItem()} {...NO_OP_MAPS} coordinatorName="Planner" />,
    );
    expect(screen.getByText("Because coverage is thin")).not.toBeNull();
    expect(screen.getByText("Approve, edit or reject")).not.toBeNull();
  });
});

describe("NeedsYouItemCard - actions by kind", () => {
  it("offers only Open task for a question item", () => {
    renderCard(
      <NeedsYouItemCard item={questionItem()} {...NO_OP_MAPS} coordinatorName="Planner" />,
    );
    expect(screen.getByRole("link", { name: "Open task" })).not.toBeNull();
    expect(screen.queryByText(SHOW_THE_EVIDENCE)).toBeNull();
  });

  it("offers only Open task for an error item", () => {
    renderCard(<NeedsYouItemCard item={errorItem()} {...NO_OP_MAPS} coordinatorName="Planner" />);
    expect(screen.getByRole("link", { name: "Open task" })).not.toBeNull();
    expect(screen.queryByText(SHOW_THE_EVIDENCE)).toBeNull();
  });

  it("offers Open task and Show the evidence for a stall item", () => {
    renderCard(<NeedsYouItemCard item={stallItem()} {...NO_OP_MAPS} coordinatorName="Planner" />);
    expect(screen.getByRole("link", { name: "Open task" })).not.toBeNull();
    expect(screen.getByText(SHOW_THE_EVIDENCE)).not.toBeNull();
  });

  it("offers no Open task/Show the evidence footer for a proposal", () => {
    renderCard(
      <NeedsYouItemCard item={proposalItem()} {...NO_OP_MAPS} coordinatorName="Planner" />,
    );
    expect(screen.queryByRole("link", { name: "Open task" })).toBeNull();
    expect(screen.queryByText(SHOW_THE_EVIDENCE)).toBeNull();
  });

  it("disables Ask about this for a reader, with a tooltip and no handler", () => {
    renderCard(
      <NeedsYouItemCard
        item={questionItem()}
        {...NO_OP_MAPS}
        canManage={false}
        coordinatorName="Planner"
      />,
    );
    const button = screen.getByRole("button", { name: ASK_ABOUT_THIS });
    expect(button.hasAttribute("disabled")).toBe(true);
    fireEvent.click(button);
    expect(useCopilotStore.getState().entries["co-1"]).toBeUndefined();
  });

  it("enables Ask about this for a manager on every kind", () => {
    for (const item of [questionItem(), stallItem(), errorItem(), proposalItem()]) {
      const { unmount } = renderCard(
        <NeedsYouItemCard item={item} {...NO_OP_MAPS} coordinatorName="Planner" />,
      );
      const button = screen.getByRole("button", { name: ASK_ABOUT_THIS });
      expect(button.hasAttribute("disabled")).toBe(false);
      unmount();
    }
  });

  it("opens the copilot with the derived id and the translated question, on click", () => {
    renderCard(
      <NeedsYouItemCard
        item={questionItem({ task: task("t-1", { identifier: "KAN-1" }) })}
        {...NO_OP_MAPS}
        coordinatorName="Planner"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: ASK_ABOUT_THIS }));
    expect(useCopilotStore.getState().entries["co-1"]).toEqual({
      open: true,
      chip: { id: "KAN-1", label: "KAN-1", ref: { kind: "task", id: "t-1" } },
      draft: "Why is KAN-1 here?",
    });
  });

  it("derives the id from the proposal's own title when it has no source task, and refs the proposal id", () => {
    const withoutSource = proposalItem();
    withoutSource.proposal.spec.source_task_id = "";
    withoutSource.proposal.spec.title = "New feature";
    renderCard(<NeedsYouItemCard item={withoutSource} {...NO_OP_MAPS} coordinatorName="Planner" />);
    fireEvent.click(screen.getByRole("button", { name: ASK_ABOUT_THIS }));
    expect(useCopilotStore.getState().entries["co-1"]?.chip).toEqual({
      id: "New feature",
      label: "New feature",
      ref: { kind: "proposal", id: "p-1" },
    });
  });

  it("refs the task id, as kind stall, for a stall item", () => {
    render(<NeedsYouItemCard item={stallItem()} {...NO_OP_MAPS} coordinatorName="Planner" />);
    fireEvent.click(screen.getByRole("button", { name: ASK_ABOUT_THIS }));
    expect(useCopilotStore.getState().entries["co-1"]?.chip?.ref).toEqual({
      kind: "stall",
      id: "t-2",
    });
  });
});

describe("NeedsYouItemCard - proposal card", () => {
  it("uses 'New task' with no step when the source task is absent from the open tasks", () => {
    renderCard(
      <NeedsYouItemCard item={proposalItem()} {...NO_OP_MAPS} coordinatorName="Planner" />,
    );
    expect(screen.getByText("New task")).not.toBeNull();
  });

  it("uses the source task's identifier and step when it is an open task", () => {
    renderCard(
      <NeedsYouItemCard
        item={proposalItem()}
        {...NO_OP_MAPS}
        openTasksById={new Map([["t-4", task("t-4", { identifier: "KAN-4" })]])}
        stepNameByTaskId={new Map([["t-4", "QA"]])}
        coordinatorName="Planner"
      />,
    );
    expect(screen.getByText("KAN-4")).not.toBeNull();
    expect(screen.getByText("QA")).not.toBeNull();
  });

  it("renders nothing for the row source when the store has no matching row yet", () => {
    renderCard(
      <NeedsYouItemCard item={proposalItem()} {...NO_OP_MAPS} coordinatorName="Planner" />,
    );
    expect(screen.queryByTestId("proposal-card-p-1")).toBeNull();
  });

  it("reads its full row from the store and renders the title, description, workflow/step, attribution and policy line", () => {
    seedProposal(COORDINATOR_ID, proposalRow());
    renderCard(
      <NeedsYouItemCard
        item={proposalItem()}
        {...NO_OP_MAPS}
        workflowNameById={new Map([["wf-1", "Planner workflow"]])}
        stepNameByWorkflowStep={new Map([["wf-1:step-1", "Build"]])}
        coordinatorName="Planner"
      />,
    );

    expect(screen.getByTestId("proposal-card-p-1")).not.toBeNull();
    expect(screen.getByText("Add tests")).not.toBeNull();
    expect(screen.getByText("Coverage is thin here")).not.toBeNull();
    expect(screen.getByText("Planner workflow · Build")).not.toBeNull();
    expect(screen.getByText("Proposed by Planner")).not.toBeNull();
    expect(screen.getByText("Policy: Create a card is propose-only")).not.toBeNull();
  });

  it("renders Approve, Edit and Reject for a manager", () => {
    seedProposal(COORDINATOR_ID, proposalRow());
    renderCard(
      <NeedsYouItemCard
        item={proposalItem()}
        {...NO_OP_MAPS}
        canManage
        coordinatorName="Planner"
      />,
    );
    expect(screen.getByRole("button", { name: "Approve" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Edit" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Reject" })).not.toBeNull();
  });

  it("renders no decision actions for a reader", () => {
    seedProposal(COORDINATOR_ID, proposalRow());
    renderCard(
      <NeedsYouItemCard
        item={proposalItem()}
        {...NO_OP_MAPS}
        canManage={false}
        coordinatorName="Planner"
      />,
    );
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reject" })).toBeNull();
  });
});

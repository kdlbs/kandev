import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type { Proposal, ProposalSpec } from "@/lib/api/domains/coordinator-api";
import type {
  ProposalDecisionOutcome,
  UseProposalDecisionResult,
} from "@/hooks/domains/coordinator/use-proposal-decision";
import type { UseProposalEditOptionsResult } from "@/hooks/domains/coordinator/use-proposal-edit-options";

const decisionState = vi.hoisted(() => ({
  current: {
    busy: false,
    approve: vi.fn(),
    reject: vi.fn(),
  } as UseProposalDecisionResult,
}));

vi.mock("@/hooks/domains/coordinator/use-proposal-decision", () => ({
  useProposalDecision: () => decisionState.current,
}));

const editOptions = vi.hoisted(() => ({ current: undefined as unknown }));

vi.mock("@/hooks/domains/coordinator/use-proposal-edit-options", () => ({
  useProposalEditOptions: () => editOptions.current,
}));

import { ProposalCard, type ProposalCardProps } from "./proposal-card";

afterEach(() => {
  cleanup();
  decisionState.current = { busy: false, approve: vi.fn(), reject: vi.fn() };
  editOptions.current = undefined;
});

function spec(overrides: Partial<ProposalSpec> = {}): ProposalSpec {
  return {
    title: "Add tests",
    description: "Cover the new endpoint",
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
    id: "p-1",
    coordinator_id: "c-1",
    workspace_id: "w-1",
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

function loadedEditOptions(
  overrides: Partial<UseProposalEditOptionsResult> = {},
): UseProposalEditOptionsResult {
  return {
    workflows: {
      status: "loaded",
      value: [{ id: "wf-1", workspace_id: "w-1", name: "Build" }] as never,
    },
    steps: {
      status: "loaded",
      value: [{ id: "step-1", name: "Review", is_start_step: true }] as never,
    },
    repositories: {
      status: "loaded",
      value: [{ id: "repo-1", workspace_id: "w-1", name: "app" }] as never,
    },
    retryWorkflows: vi.fn(),
    retryRepositories: vi.fn(),
    retrySteps: vi.fn(),
    ...overrides,
  };
}

const WORKFLOW_NAMES = new Map([["wf-1", "Build"]]);
const STEP_NAMES = new Map([["wf-1:step-1", "Review"]]);
const CARD_TEST_ID = "proposal-card-p-1";
const TOAST_TEST_ID = "toast-message";
const REASON_LABEL = "Reason (optional)";
const VALIDATION_MESSAGE = "Enter a title";

function renderCard(overrides: Partial<ProposalCardProps> = {}) {
  const props: ProposalCardProps = {
    proposal: proposal(),
    variant: "full",
    canManage: true,
    workspaceId: "w-1",
    coordinatorId: "c-1",
    workflowNameById: WORKFLOW_NAMES,
    stepNameByWorkflowStep: STEP_NAMES,
    coordinatorName: "Coordinator",
    ...overrides,
  };
  return render(
    <ToastProvider>
      <ProposalCard {...props} />
    </ToastProvider>,
  );
}

function statusText(testId: string) {
  return screen.getByTestId(testId).querySelector('p[class*="text-sm"]')?.textContent;
}

describe("ProposalCard - content by state", () => {
  it("shows the pending line", () => {
    renderCard({ proposal: proposal({ status: "pending" }) });
    expect(statusText(CARD_TEST_ID)).toBe("Pending approval");
  });

  it("shows the approving line", () => {
    renderCard({ proposal: proposal({ status: "approving" }) });
    expect(statusText(CARD_TEST_ID)).toBe("Approval in progress. Edits are locked.");
  });

  it("shows the failed line with the server error", () => {
    renderCard({ proposal: proposal({ status: "failed", error: "no capacity" }) });
    expect(statusText(CARD_TEST_ID)).toBe(
      "Could not create the task: no capacity. Nothing was created.",
    );
  });

  it("shows the failed line with no error", () => {
    renderCard({ proposal: proposal({ status: "failed", error: null }) });
    expect(statusText(CARD_TEST_ID)).toBe("Could not create the task. Nothing was created.");
  });

  it("shows the rejected line with a reason", () => {
    renderCard({ proposal: proposal({ status: "rejected", reject_reason: "Not now" }) });
    expect(statusText(CARD_TEST_ID)).toBe("Rejected: Not now");
  });

  it("shows the rejected line with no reason", () => {
    renderCard({ proposal: proposal({ status: "rejected", reject_reason: null }) });
    expect(statusText(CARD_TEST_ID)).toBe("Rejected");
  });

  it("shows the approved line, falling back to the spec title with no task_id", () => {
    renderCard({ proposal: proposal({ status: "approved", task_id: null }) });
    expect(statusText(CARD_TEST_ID)).toBe("Approved: Add tests");
  });
});

describe("ProposalCard - busy announcement", () => {
  it("announces Working in the live region while busy, instead of the status line", () => {
    decisionState.current = { busy: true, approve: vi.fn(), reject: vi.fn() };
    renderCard({ proposal: proposal({ status: "pending" }) });
    const live = screen.getByTestId(CARD_TEST_ID).querySelector('[role="status"]');
    expect(live?.textContent).toBe("Working");
  });

  it("announces the status line in the live region when not busy", () => {
    renderCard({ proposal: proposal({ status: "pending" }) });
    const live = screen.getByTestId(CARD_TEST_ID).querySelector('[role="status"]');
    expect(live?.textContent).toBe("Pending approval");
  });
});

describe("ProposalCard - actions gating", () => {
  it("shows no actions when canManage is false", () => {
    renderCard({ canManage: false, proposal: proposal({ status: "pending" }) });
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
  });

  it.each(["approving", "approved", "rejected"] as const)(
    "shows no actions for status %s even when canManage is true",
    (status) => {
      renderCard({ proposal: proposal({ status }) });
      expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    },
  );

  it.each(["pending", "failed"] as const)(
    "shows actions for status %s when canManage is true",
    (status) => {
      renderCard({ proposal: proposal({ status }) });
      expect(screen.getByRole("button", { name: "Approve" })).not.toBeNull();
      expect(screen.getByRole("button", { name: "Edit" })).not.toBeNull();
      expect(screen.getByRole("button", { name: "Reject" })).not.toBeNull();
    },
  );
});

describe("ProposalCard - full variant", () => {
  it("shows description, attribution and policy lines", () => {
    renderCard({ variant: "full" });
    const card = screen.getByTestId(CARD_TEST_ID);
    expect(within(card).getByText("Cover the new endpoint")).not.toBeNull();
    expect(within(card).getByText("Proposed by Coordinator")).not.toBeNull();
  });

  it("opens the edit form inline on Edit", () => {
    editOptions.current = loadedEditOptions();
    renderCard({ variant: "full" });
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(screen.getByLabelText("Title")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
  });

  it("opens the reject form inline on Reject", () => {
    renderCard({ variant: "full" });
    fireEvent.click(screen.getByRole("button", { name: "Reject" }));
    expect(screen.getByLabelText(REASON_LABEL)).not.toBeNull();
  });

  it("returns focus to Edit when the edit form is cancelled", () => {
    editOptions.current = loadedEditOptions();
    renderCard({ variant: "full" });
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Edit" }));
  });
});

describe("ProposalCard - compact variant", () => {
  it("does not show description, attribution or policy lines", () => {
    renderCard({ variant: "compact" });
    const card = screen.getByTestId(CARD_TEST_ID);
    expect(within(card).queryByText("Cover the new endpoint")).toBeNull();
    expect(within(card).queryByText("Proposed by Coordinator")).toBeNull();
  });

  it("navigates to Needs-you instead of opening an inline form on Edit", () => {
    const onNavigateToForm = vi.fn();
    renderCard({ variant: "compact", onNavigateToForm });
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(onNavigateToForm).toHaveBeenCalledWith("edit");
    expect(screen.queryByLabelText("Title")).toBeNull();
  });

  it("navigates to Needs-you instead of opening an inline form on Reject", () => {
    const onNavigateToForm = vi.fn();
    renderCard({ variant: "compact", onNavigateToForm });
    fireEvent.click(screen.getByRole("button", { name: "Reject" }));
    expect(onNavigateToForm).toHaveBeenCalledWith("reject");
    expect(screen.queryByLabelText(REASON_LABEL)).toBeNull();
  });
});

describe("ProposalCard - decision outcomes", () => {
  function withOutcome(outcome: ProposalDecisionOutcome) {
    decisionState.current = {
      busy: false,
      approve: vi.fn().mockResolvedValue(outcome),
      reject: vi.fn().mockResolvedValue(outcome),
    };
  }

  it("toasts the approved copy with the Next-count line on a decided/approved outcome", async () => {
    withOutcome({ kind: "decided", proposal: proposal({ status: "approved" }) });
    renderCard({ computeNeedsYouCount: () => 2 });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    const toast = await screen.findByTestId(TOAST_TEST_ID);
    expect(toast.textContent).toContain("Add tests");
    expect(toast.textContent).toContain("2 items still need you");
  });

  it("shows the nothing-needs-you line when the count is zero", async () => {
    withOutcome({ kind: "decided", proposal: proposal({ status: "approved" }) });
    renderCard({ computeNeedsYouCount: () => 0 });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    const toast = await screen.findByTestId(TOAST_TEST_ID);
    expect(toast.textContent).toContain("nothing needs you");
  });

  it("toasts the rejected copy on a decided/rejected outcome", async () => {
    withOutcome({ kind: "decided", proposal: proposal({ status: "rejected" }) });
    renderCard();
    fireEvent.click(screen.getByRole("button", { name: "Reject" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm reject" }));
    const toast = await screen.findByTestId(TOAST_TEST_ID);
    expect(toast.textContent).toContain("Rejected. Nothing was created");
  });

  it("does not toast on a decided/failed outcome", async () => {
    withOutcome({ kind: "decided", proposal: proposal({ status: "failed", error: "boom" }) });
    renderCard();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await Promise.resolve();
    expect(screen.queryByTestId(TOAST_TEST_ID)).toBeNull();
  });

  it("full variant: opens the edit form and shows the field error for a plain-approve validation outcome", async () => {
    editOptions.current = loadedEditOptions();
    withOutcome({ kind: "validation", message: VALIDATION_MESSAGE, field: "title" });
    renderCard({ variant: "full" });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    expect(await screen.findByLabelText("Title")).not.toBeNull();
    expect(screen.getByText(VALIDATION_MESSAGE)).not.toBeNull();
  });

  it("compact variant: keeps the buttons visible and shows the error text for a plain-approve validation outcome", async () => {
    withOutcome({ kind: "validation", message: VALIDATION_MESSAGE, field: "title" });
    renderCard({ variant: "compact" });
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    expect(await screen.findByText(VALIDATION_MESSAGE)).not.toBeNull();
    expect(screen.getByRole("button", { name: "Approve" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Edit" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Reject" })).not.toBeNull();
  });

  it("toasts a conflict outcome and closes any open form", async () => {
    editOptions.current = loadedEditOptions();
    withOutcome({ kind: "conflict", proposal: proposal({ status: "approving" }) });
    renderCard({ variant: "full" });
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.click(screen.getByRole("button", { name: "Approve with edits" }));
    const toast = await screen.findByTestId(TOAST_TEST_ID);
    expect(toast.textContent).toContain("Someone else already decided this proposal.");
    expect(screen.queryByLabelText("Title")).toBeNull();
  });

  it("toasts a forbidden outcome", async () => {
    withOutcome({ kind: "forbidden" });
    renderCard();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    const toast = await screen.findByTestId(TOAST_TEST_ID);
    expect(toast.textContent).toContain("You no longer have permission to decide proposals.");
  });

  it("toasts a not_found outcome", async () => {
    withOutcome({ kind: "not_found" });
    renderCard();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    const toast = await screen.findByTestId(TOAST_TEST_ID);
    expect(toast.textContent).toContain("This proposal no longer exists.");
  });

  it("toasts a network outcome", async () => {
    withOutcome({ kind: "network" });
    renderCard();
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    const toast = await screen.findByTestId(TOAST_TEST_ID);
    expect(toast.textContent).toContain("Could not reach Kandev. Nothing was decided. Try again.");
  });
});

describe("ProposalCard - remote changes while a form is open", () => {
  it("keeps the form open and the typed title when a remote merge keeps the proposal pending", () => {
    editOptions.current = loadedEditOptions();
    const onFormForceClosed = vi.fn();
    const { rerender } = renderCard({
      variant: "full",
      proposal: proposal({ status: "pending" }),
      onFormForceClosed,
    });
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Typed title" } });

    rerender(
      <ToastProvider>
        <ProposalCard
          proposal={proposal({ status: "pending", updated_at: "2026-09-27T00:01:00Z" })}
          variant="full"
          canManage
          workspaceId="w-1"
          coordinatorId="c-1"
          workflowNameById={WORKFLOW_NAMES}
          stepNameByWorkflowStep={STEP_NAMES}
          coordinatorName="Coordinator"
          onFormForceClosed={onFormForceClosed}
        />
      </ToastProvider>,
    );

    expect(screen.getByLabelText("Title")).not.toBeNull();
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe("Typed title");
    expect(onFormForceClosed).not.toHaveBeenCalled();
  });

  it("force-closes an open form when the proposal becomes approving remotely, and notifies the caller", () => {
    editOptions.current = loadedEditOptions();
    const onFormForceClosed = vi.fn();
    const { rerender } = renderCard({
      variant: "full",
      proposal: proposal({ status: "pending" }),
      onFormForceClosed,
    });
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(screen.getByLabelText("Title")).not.toBeNull();

    rerender(
      <ToastProvider>
        <ProposalCard
          proposal={proposal({ status: "approving" })}
          variant="full"
          canManage
          workspaceId="w-1"
          coordinatorId="c-1"
          workflowNameById={WORKFLOW_NAMES}
          stepNameByWorkflowStep={STEP_NAMES}
          coordinatorName="Coordinator"
          onFormForceClosed={onFormForceClosed}
        />
      </ToastProvider>,
    );

    expect(screen.queryByLabelText("Title")).toBeNull();
    expect(onFormForceClosed).toHaveBeenCalledOnce();
  });

  it("does not force-close a form this card's own approve put into approving", () => {
    editOptions.current = loadedEditOptions();
    const onFormForceClosed = vi.fn();
    decisionState.current = {
      busy: true,
      approve: vi.fn(() => new Promise<ProposalDecisionOutcome>(() => {})),
      reject: vi.fn(),
    };
    const { rerender } = renderCard({
      variant: "full",
      proposal: proposal({ status: "pending" }),
      onFormForceClosed,
    });
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    rerender(
      <ToastProvider>
        <ProposalCard
          proposal={proposal({ status: "approving" })}
          variant="full"
          canManage
          workspaceId="w-1"
          coordinatorId="c-1"
          workflowNameById={WORKFLOW_NAMES}
          stepNameByWorkflowStep={STEP_NAMES}
          coordinatorName="Coordinator"
          onFormForceClosed={onFormForceClosed}
        />
      </ToastProvider>,
    );

    expect(onFormForceClosed).not.toHaveBeenCalled();
  });
});

describe("ProposalCard - auto-opening a form on mount", () => {
  it("opens the edit form on mount when autoOpenForm is edit, and reports it once", () => {
    editOptions.current = loadedEditOptions();
    const onAutoFormOpened = vi.fn();
    renderCard({ variant: "full", autoOpenForm: "edit", onAutoFormOpened });
    expect(screen.getByLabelText("Title")).not.toBeNull();
    expect(onAutoFormOpened).toHaveBeenCalledOnce();
  });

  it("opens the reject form on mount when autoOpenForm is reject", () => {
    const onAutoFormOpened = vi.fn();
    renderCard({ variant: "full", autoOpenForm: "reject", onAutoFormOpened });
    expect(screen.getByLabelText(REASON_LABEL)).not.toBeNull();
    expect(onAutoFormOpened).toHaveBeenCalledOnce();
  });

  it("does not open a form on mount when autoOpenForm is absent", () => {
    renderCard({ variant: "full" });
    expect(screen.queryByLabelText("Title")).toBeNull();
    expect(screen.queryByLabelText(REASON_LABEL)).toBeNull();
  });

  it("opens the form when autoOpenForm arrives on a later render of an already-mounted card", () => {
    // The Needs-you deep link's target item can reach the list (and mount
    // this card with autoOpenForm still null) before every other
    // coordinator input has loaded; only once loading finishes does the
    // hook start returning a non-null autoOpenForm, on a render of a card
    // that already exists.
    editOptions.current = loadedEditOptions();
    const onAutoFormOpened = vi.fn();
    const { rerender } = renderCard({ variant: "full", autoOpenForm: null, onAutoFormOpened });
    expect(screen.queryByLabelText("Title")).toBeNull();
    expect(onAutoFormOpened).not.toHaveBeenCalled();

    rerender(
      <ToastProvider>
        <ProposalCard
          proposal={proposal()}
          variant="full"
          canManage={true}
          workspaceId="w-1"
          coordinatorId="c-1"
          workflowNameById={WORKFLOW_NAMES}
          stepNameByWorkflowStep={STEP_NAMES}
          coordinatorName="Coordinator"
          autoOpenForm="edit"
          onAutoFormOpened={onAutoFormOpened}
        />
      </ToastProvider>,
    );

    expect(screen.getByLabelText("Title")).not.toBeNull();
    expect(onAutoFormOpened).toHaveBeenCalledOnce();
  });
});

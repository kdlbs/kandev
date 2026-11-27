import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type { Proposal, StoredProposal } from "@/lib/api/domains/coordinator-api";
import type { ProposalDecisionOutcome } from "@/hooks/domains/coordinator/use-proposal-decision";
import type { UseProposalReplyResult } from "@/hooks/domains/coordinator/use-proposal-reply";

const phase3 = vi.hoisted(() => ({ effective: true }));
vi.mock("@/hooks/domains/settings/use-coordinator-phase3-effective", () => ({
  useCoordinatorPhase3Effective: () => phase3.effective,
}));

const replyState = vi.hoisted(() => ({ current: undefined as unknown }));
vi.mock("@/hooks/domains/coordinator/use-proposal-reply", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/hooks/domains/coordinator/use-proposal-reply")>();
  return { ...actual, useProposalReply: () => replyState.current };
});

vi.mock("@/hooks/domains/coordinator/use-proposal-decision", () => ({
  useProposalDecision: () => ({ busy: false, approve: vi.fn(), reject: vi.fn() }),
}));

const original = vi.hoisted(() => ({ row: undefined as unknown }));
vi.mock("@/hooks/domains/coordinator/use-proposals", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/hooks/domains/coordinator/use-proposals")>();
  return { ...actual, useProposalById: () => ({ proposal: original.row, notFound: false }) };
});

vi.mock("../use-now-tick", () => ({ useNowTick: () => Date.parse("2026-09-27T00:10:00Z") }));
vi.mock("@/lib/api/domains/kanban-api", () => ({ fetchTask: vi.fn() }));

import { ProposalCard, type ProposalCardProps } from "./proposal-card";

const REPLY_BUTTON = "Reply with a condition";
const RETURNED_LINE = "Returned with your condition: narrower";
const TOAST_ID = "toast-message";
const CONDITION_LABEL = "Your condition";
const SEND_REPLY = "Send reply";
const SEND_AGAIN = "Send again";

function proposal(overrides: Partial<Proposal> = {}): Proposal {
  return {
    id: "p-1",
    coordinator_id: "c-1",
    workspace_id: "w-1",
    status: "pending",
    spec: {
      title: "Add tests",
      description: "desc",
      rationale: "why",
      workflow_id: "wf-1",
      step_id: "step-1",
      repository_id: "repo-1",
      source_task_id: "t-1",
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
  };
}

function reply(overrides: Partial<UseProposalReplyResult> = {}): UseProposalReplyResult {
  return { busy: false, reply: vi.fn(), redeliver: vi.fn(), ...overrides };
}

function renderCard(p: StoredProposal, overrides: Partial<ProposalCardProps> = {}) {
  return render(
    <ToastProvider>
      <ProposalCard
        proposal={p}
        variant="full"
        canManage
        workspaceId="w-1"
        coordinatorId="c-1"
        workflowNameById={new Map()}
        stepNameByWorkflowStep={new Map()}
        coordinatorName="Coordinator"
        {...overrides}
      />
    </ToastProvider>,
  );
}

beforeEach(() => {
  phase3.effective = true;
  replyState.current = reply();
  original.row = undefined;
});
afterEach(cleanup);

describe("ProposalCard reply control", () => {
  it("offers Reply with a condition on a pending proposal for a manager", () => {
    renderCard(proposal());
    expect(screen.getByRole("button", { name: REPLY_BUTTON })).toBeTruthy();
  });

  it.each([
    ["phase 3 is not effective", () => (phase3.effective = false), proposal()],
    ["the proposal failed", () => undefined, proposal({ status: "failed" })],
  ])("hides the control when %s", (_name, arrange, p) => {
    arrange();
    renderCard(p);
    expect(screen.queryByRole("button", { name: REPLY_BUTTON })).toBeNull();
  });

  it("hides the control from a non-manager", () => {
    renderCard(proposal(), { canManage: false });
    expect(screen.queryByRole("button", { name: REPLY_BUTTON })).toBeNull();
  });

  it("opens in place on the compact chat card", () => {
    renderCard(proposal(), { variant: "compact" });
    fireEvent.click(screen.getByRole("button", { name: REPLY_BUTTON }));
    expect(screen.getByLabelText(CONDITION_LABEL)).toBeTruthy();
  });

  it("returns focus to the control on Cancel", async () => {
    renderCard(proposal());
    fireEvent.click(screen.getByRole("button", { name: REPLY_BUTTON }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("button", { name: REPLY_BUTTON })),
    );
  });

  it("holds the item and toasts once a delivered reply lands", async () => {
    const outcome: ProposalDecisionOutcome = {
      kind: "decided",
      proposal: proposal({
        status: "returned",
        reply_delivered_at: "2026-09-27T00:01:00Z",
      }) as StoredProposal,
    };
    const send = vi.fn().mockResolvedValue(outcome);
    replyState.current = reply({ reply: send });
    const onReplyStarted = vi.fn();
    renderCard(proposal(), { onReplyStarted });
    fireEvent.click(screen.getByRole("button", { name: REPLY_BUTTON }));
    fireEvent.change(screen.getByLabelText(CONDITION_LABEL), { target: { value: " narrower " } });
    fireEvent.click(screen.getByRole("button", { name: SEND_REPLY }));
    expect(onReplyStarted).toHaveBeenCalledOnce();
    expect(send).toHaveBeenCalledWith("narrower");
    await waitFor(() =>
      expect(screen.getByTestId(TOAST_ID).textContent).toBe("Reply sent to the coordinator."),
    );
    expect(screen.queryByLabelText(CONDITION_LABEL)).toBeNull();
  });

  it("keeps the form and its text open after a network failure", async () => {
    replyState.current = reply({ reply: vi.fn().mockResolvedValue({ kind: "network" }) });
    renderCard(proposal());
    fireEvent.click(screen.getByRole("button", { name: REPLY_BUTTON }));
    fireEvent.change(screen.getByLabelText(CONDITION_LABEL), { target: { value: "keep me" } });
    fireEvent.click(screen.getByRole("button", { name: SEND_REPLY }));
    await waitFor(() =>
      expect(screen.getByTestId(TOAST_ID).textContent).toContain("may not have been saved"),
    );
    expect((screen.getByLabelText(CONDITION_LABEL) as HTMLTextAreaElement).value).toBe("keep me");
  });

  it("keeps the form open with an inline error on a 400 naming text", async () => {
    replyState.current = reply({
      reply: vi
        .fn()
        .mockResolvedValue({ kind: "validation", message: "text is too long", field: "text" }),
    });
    renderCard(proposal());
    fireEvent.click(screen.getByRole("button", { name: REPLY_BUTTON }));
    fireEvent.change(screen.getByLabelText(CONDITION_LABEL), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: SEND_REPLY }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toBe("text is too long"));
  });

  it("closes the form with a toast on a 400 naming kind", async () => {
    replyState.current = reply({
      reply: vi.fn().mockResolvedValue({ kind: "validation", message: "kind", field: "kind" }),
    });
    renderCard(proposal());
    fireEvent.click(screen.getByRole("button", { name: REPLY_BUTTON }));
    fireEvent.change(screen.getByLabelText(CONDITION_LABEL), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: SEND_REPLY }));
    await waitFor(() =>
      expect(screen.getByTestId(TOAST_ID).textContent).toBe("This proposal cannot be replied to."),
    );
    expect(screen.queryByLabelText(CONDITION_LABEL)).toBeNull();
  });
});

describe("ProposalCard returned states", () => {
  const returned = (overrides: Partial<Proposal> = {}) =>
    proposal({ status: "returned", reply_text: "narrower", ...overrides });

  it("shows the condition and Send again while the reply is not delivered", async () => {
    const redeliver = vi.fn().mockResolvedValue({ kind: "network" });
    replyState.current = reply({ redeliver });
    renderCard(returned());
    expect(screen.getAllByText(RETURNED_LINE).length).toBeGreaterThan(0);
    expect(screen.getAllByText("Reply saved, not delivered").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: SEND_AGAIN }));
    expect(redeliver).toHaveBeenCalledOnce();
    await waitFor(() =>
      expect(screen.getByTestId(TOAST_ID).textContent).toBe("Could not reach Kandev. Try again."),
    );
  });

  it("shows Sending reply and disables Send again while in flight", () => {
    replyState.current = reply({ busy: true });
    renderCard(returned());
    expect(screen.getByText("Sending reply")).toBeTruthy();
    expect((screen.getByRole("button", { name: /Send again/ }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("offers no Send again once delivered", () => {
    renderCard(returned({ reply_delivered_at: "2026-09-27T00:01:00Z" }));
    expect(screen.queryByRole("button", { name: SEND_AGAIN })).toBeNull();
    expect(screen.queryByText("Reply saved, not delivered")).toBeNull();
  });

  it("shows a settled line without text or Send again when phase 3 is off", () => {
    phase3.effective = false;
    renderCard(returned({ reply_text: undefined, reply_delivered_at: undefined }));
    expect(screen.getAllByText("Returned with your condition").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: SEND_AGAIN })).toBeNull();
    expect(screen.queryByText("Reply saved, not delivered")).toBeNull();
  });

  it("shows the returned line to a non-manager without controls", () => {
    renderCard(returned(), { canManage: false });
    expect(screen.getAllByText(RETURNED_LINE).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: SEND_AGAIN })).toBeNull();
  });
});

describe("ProposalCard revised note", () => {
  it("quotes the reply of the proposal it revises", () => {
    original.row = proposal({ id: "p-0", status: "returned", reply_text: "narrower" });
    renderCard(proposal({ id: "p-2", in_reply_to: "p-0" }));
    expect(screen.getByText("Revised after your reply")).toBeTruthy();
    expect(screen.getByText("Your reply: narrower")).toBeTruthy();
  });

  it("shows only the heading while the original is unreadable", () => {
    original.row = undefined;
    renderCard(proposal({ id: "p-2", in_reply_to: "p-0" }));
    expect(screen.getByText("Revised after your reply")).toBeTruthy();
    expect(screen.queryByText(/Your reply:/)).toBeNull();
  });

  it("shows no note for an ordinary proposal", () => {
    renderCard(proposal());
    expect(screen.queryByTestId("proposal-revised-note")).toBeNull();
  });
});

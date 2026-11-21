import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type { AnswerProposal, AnswerSpec } from "@/lib/api/domains/coordinator-api";
import type { UseProposalDecisionResult } from "@/hooks/domains/coordinator/use-proposal-decision";

const decisionState = vi.hoisted(() => ({
  current: {
    busy: false,
    approve: vi.fn(),
    reject: vi.fn(),
  } as UseProposalDecisionResult,
}));
const flag = vi.hoisted(() => ({ on: true }));
const relayState = vi.hoisted(() => ({ read: vi.fn() }));

vi.mock("@/hooks/domains/coordinator/use-proposal-decision", () => ({
  useProposalDecision: () => decisionState.current,
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => flag.on }));
vi.mock("@/hooks/domains/settings/use-coordinator-phase3-effective", () => ({
  useCoordinatorPhase3Effective: () => true,
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ fetchTask: vi.fn() }));
vi.mock("@/lib/api/domains/coordinator-relay-api", () => ({
  readRelay: (...args: unknown[]) => relayState.read(...args),
}));
vi.mock("../use-now-tick", () => ({
  useNowTick: () => new Date("2026-09-27T00:10:00Z").getTime(),
}));

import { ProposalCard, type ProposalCardProps } from "./proposal-card";
import { Phase2CardProvider } from "./phase2-context";

afterEach(() => {
  cleanup();
  flag.on = true;
  relayState.read.mockReset();
  decisionState.current = { busy: false, approve: vi.fn(), reject: vi.fn() };
});

const CLARIFICATION: AnswerSpec = {
  task_id: "t-1",
  target: "clarification",
  pending_id: "pend-1",
  draft: { answers: [{ question_id: "q1", option_ids: ["a"], custom_text: "because" }] },
  snapshot: {
    questions: [
      {
        id: "q1",
        text: "Which database?",
        options: [
          { id: "a", label: "Postgres" },
          { id: "b", label: "SQLite" },
        ],
      },
    ],
  },
  sources: [
    { ref: "https://example.com/doc", label: "Design doc" },
    { ref: "docs/db.md:12", label: "Notes" },
    { ref: "javascript:alert(1)" },
  ],
  confidence: "med",
  needs_decision: true,
  rationale: "The doc says Postgres.",
};

const PERMISSION: AnswerSpec = {
  task_id: "t-1",
  target: "permission",
  pending_id: "pend-2",
  request_id: "req-1",
  draft: { option_id: "allow" },
  snapshot: {
    permission: {
      title: "Run ls",
      summary: "list files",
      options: [
        { id: "allow", name: "Allow once", kind: "allow_once" },
        { id: "deny", name: "Reject once", kind: "reject_once" },
        { id: "always", name: "Always", kind: "allow_always" },
      ],
    },
  },
  sources: [],
  confidence: "high",
  needs_decision: false,
  rationale: "",
};

function row(spec: AnswerSpec, overrides: Record<string, unknown> = {}): AnswerProposal {
  return {
    id: "p-1",
    coordinator_id: "c-1",
    workspace_id: "w-1",
    status: "pending",
    kind: "answer",
    spec,
    final_spec: null,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  } as AnswerProposal;
}

function renderCard(proposal: AnswerProposal, overrides: Partial<ProposalCardProps> = {}) {
  const props: ProposalCardProps = {
    proposal,
    variant: "full",
    canManage: true,
    workspaceId: "w-1",
    coordinatorId: "c-1",
    workflowNameById: new Map(),
    stepNameByWorkflowStep: new Map(),
    openTasksById: new Map([["t-1", { id: "t-1", title: "Fix login", identifier: "KAN-418" }]]),
    coordinatorName: "Coordinator",
    ...overrides,
  };
  return render(
    <ToastProvider>
      <Phase2CardProvider value={{ enabled: true, orders: undefined, offerReject: undefined }}>
        <ProposalCard {...props} />
      </Phase2CardProvider>
    </ToastProvider>,
  );
}

describe("answer card content", () => {
  it("shows the question, draft, confidence, badge and sources", () => {
    renderCard(row(CLARIFICATION));
    expect(screen.getByText(/Which database\?/)).not.toBeNull();
    expect(screen.getByText(/Postgres, because/)).not.toBeNull();
    expect(screen.getByTestId("answer-confidence").textContent).toContain("medium");
    expect(screen.getByTestId("answer-needs-decision").textContent).toBe("Needs your decision");
    expect(screen.getByText("The doc says Postgres.")).not.toBeNull();
  });

  it("links only http(s) sources and shows the rest as text", () => {
    renderCard(row(CLARIFICATION));
    const link = screen.getByRole("link", { name: "Design doc" });
    expect(link.getAttribute("href")).toBe("https://example.com/doc");
    expect(link.getAttribute("rel")).toContain("noopener");
    expect(screen.getByText("Notes (docs/db.md:12)")).not.toBeNull();
    expect(screen.queryByRole("link", { name: /javascript/ })).toBeNull();
    expect(screen.getAllByRole("button", { name: "Copy" })).toHaveLength(3);
  });

  it("omits the badge when the decision is not flagged", () => {
    renderCard(row(PERMISSION));
    expect(screen.queryByTestId("answer-needs-decision")).toBeNull();
    expect(screen.getByTestId("answer-permission").textContent).toContain("Run ls");
    expect(screen.getByTestId("answer-permission").textContent).toContain("Allow once");
  });

  it("renders the edited spec when one exists", () => {
    const edited = {
      ...CLARIFICATION,
      draft: { answers: [{ question_id: "q1", option_ids: ["b"] }] },
    };
    renderCard(row(CLARIFICATION, { final_spec: edited }));
    expect(screen.getByText(/SQLite/)).not.toBeNull();
  });
});

describe("answer card actions", () => {
  it("pending: Approve, Edit and Reject", () => {
    renderCard(row(CLARIFICATION));
    expect(screen.getByRole("button", { name: "Approve" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Edit" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Reject" })).not.toBeNull();
  });

  it("approve calls the decision hook without edits", async () => {
    const approve = vi.fn().mockResolvedValue({ kind: "network" });
    decisionState.current = { busy: false, approve, reject: vi.fn() };
    renderCard(row(CLARIFICATION));
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(approve).toHaveBeenCalledWith(undefined));
  });

  it("flag off: Reject only", () => {
    flag.on = false;
    renderCard(row(CLARIFICATION));
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.getByRole("button", { name: "Reject" })).not.toBeNull();
  });

  it.each([
    ["resolution_in_progress", ["Retry", "Reject"], ["Edit", "Approve"]],
    ["not_delivered", ["Retry", "Reject"], ["Edit", "Approve"]],
    ["invalid_draft", ["Edit", "Reject"], ["Retry", "Approve"]],
    ["option_not_offered", ["Edit", "Reject"], ["Retry", "Approve"]],
    ["outcome_unknown", ["Reject"], ["Retry", "Edit", "Approve"]],
  ])("failed %s offers %j and not %j", (code, shown, hidden) => {
    renderCard(row(CLARIFICATION, { status: "failed", error: code }));
    for (const name of shown) expect(screen.getByRole("button", { name })).not.toBeNull();
    for (const name of hidden) expect(screen.queryByRole("button", { name })).toBeNull();
  });

  it("outcome_unknown says the answer may have been delivered", () => {
    renderCard(row(CLARIFICATION, { status: "failed", error: "outcome_unknown" }));
    expect(screen.getByTestId("answer-may-have-been-delivered")).not.toBeNull();
  });

  it("a refused answer is read-only and says why", () => {
    renderCard(row(CLARIFICATION, { status: "rejected", error: "stale", decided_by: null }));
    expect(screen.getAllByText(/Refused \(stale\)/).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reject" })).toBeNull();
  });

  it("a task-unavailable refusal has its own line", () => {
    renderCard(row(CLARIFICATION, { status: "rejected", error: "task_archived" }));
    expect(screen.getAllByText(/Refused \(task unavailable\)/).length).toBeGreaterThan(0);
  });

  it("a person's rejection is not shown as a refusal", () => {
    renderCard(
      row(CLARIFICATION, {
        status: "rejected",
        error: "stale",
        decided_by: "u-1",
        reject_reason: "no",
      }),
    );
    expect(screen.queryByText(/Refused \(stale\)/)).toBeNull();
  });

  it("a viewer who cannot manage sees no buttons", () => {
    renderCard(row(CLARIFICATION), { canManage: false });
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
  });
});

describe("answer card edit", () => {
  it("opens the editor, warns when the question changed, and sends the draft", async () => {
    relayState.read.mockResolvedValue({
      kind: "ok",
      relay: { clarification: { pending_id: "other" }, permission: null },
    });
    const approve = vi.fn().mockResolvedValue({ kind: "network" });
    decisionState.current = { busy: false, approve, reject: vi.fn() };
    renderCard(row(CLARIFICATION));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(await screen.findByTestId("answer-out-of-date")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Approve with edits" }));
    await waitFor(() => expect(approve).toHaveBeenCalled());
    const edits = approve.mock.calls[0][0];
    expect(edits.draft.answers[0].question_id).toBe("q1");
  });

  it("keeps the form and the entered text open on a validation error", async () => {
    const approve = vi
      .fn()
      .mockResolvedValue({ kind: "validation", message: "invalid_draft", field: "draft" });
    decisionState.current = { busy: false, approve, reject: vi.fn() };
    relayState.read.mockResolvedValue({ kind: "failed" });
    renderCard(row(CLARIFICATION));
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    const input = await screen.findByPlaceholderText("Custom text");
    fireEvent.change(input, { target: { value: "typed" } });
    fireEvent.click(screen.getByRole("button", { name: "Approve with edits" }));
    await waitFor(() => expect(approve).toHaveBeenCalled());
    expect((screen.getByPlaceholderText("Custom text") as HTMLInputElement).value).toBe("typed");
  });

  it("compact variant navigates instead of opening the form", () => {
    const onNavigateToForm = vi.fn();
    renderCard(row(CLARIFICATION), { variant: "compact", onNavigateToForm });
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(onNavigateToForm).toHaveBeenCalledWith("edit");
  });
});

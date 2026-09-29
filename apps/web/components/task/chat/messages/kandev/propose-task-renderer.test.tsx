import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Proposal, ProposalSpec } from "@/lib/api/domains/coordinator-api";
import { CoordinatorProposalProvider } from "./coordinator-proposal-context";
import type { KandevStatus } from "./shared";

const pushMock = vi.fn();
const closePopoverMock = vi.fn();
const useProposalByIdMock = vi.fn();
const useProposalWorkflowNamesMock = vi.fn();
const proposalCardCalls = vi.hoisted(() => [] as Array<Record<string, unknown>>);

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

vi.mock("@/hooks/domains/coordinator/use-proposals", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/hooks/domains/coordinator/use-proposals")>();
  return { ...actual, useProposalById: (...args: unknown[]) => useProposalByIdMock(...args) };
});

vi.mock("@/hooks/domains/coordinator/use-proposal-workflow-names", () => ({
  useProposalWorkflowNames: (...args: unknown[]) => useProposalWorkflowNamesMock(...args),
}));

vi.mock("@/app/coordinator/proposal-card/proposal-card", () => ({
  ProposalCard: (props: Record<string, unknown>) => {
    proposalCardCalls.push(props);
    return <div data-testid={`proposal-card-${(props.proposal as Proposal).id}`} />;
  },
}));

import {
  ProposeMessageRenderer,
  ProposeMoveRenderer,
  ProposeResumeRenderer,
  ProposeTaskRenderer,
} from "./propose-task-renderer";

const WORKSPACE_ID = "ws-1";
const COORDINATOR_ID = "co-1";
const PROPOSAL_ID = "p-1";
const TITLE = "Kandev: Propose Task";

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

function renderRenderer(status: KandevStatus, result: unknown, withContext = true) {
  const ui = <ProposeTaskRenderer args={undefined} result={result} status={status} />;
  if (!withContext) return render(ui);
  return render(
    <CoordinatorProposalProvider
      value={{
        workspaceId: WORKSPACE_ID,
        coordinatorId: COORDINATOR_ID,
        closePopover: closePopoverMock,
      }}
    >
      {ui}
    </CoordinatorProposalProvider>,
  );
}

beforeEach(() => {
  pushMock.mockReset();
  closePopoverMock.mockReset();
  useProposalByIdMock.mockReset();
  useProposalByIdMock.mockReturnValue({ proposal: undefined, notFound: false });
  useProposalWorkflowNamesMock.mockReset();
  useProposalWorkflowNamesMock.mockReturnValue({
    workflowNameById: new Map(),
    stepNameByWorkflowStep: new Map(),
  });
  proposalCardCalls.length = 0;
});

afterEach(cleanup);

describe("ProposeTaskRenderer - ordinary tool-call fallback", () => {
  it("renders the plain row, no card, when the tool call's status is error", () => {
    renderRenderer("error", { proposal_id: PROPOSAL_ID });
    expect(screen.getByTestId("propose-task-renderer")).not.toBeNull();
    expect(screen.getByText(TITLE)).not.toBeNull();
    expect(screen.queryByTestId(`proposal-card-${PROPOSAL_ID}`)).toBeNull();
  });

  it("renders the plain row, no card, when the result has no string proposal_id", () => {
    renderRenderer("complete", { status: "pending" });
    expect(screen.getByText(TITLE)).not.toBeNull();
    expect(screen.queryByTestId(`proposal-card-${PROPOSAL_ID}`)).toBeNull();
  });

  it("renders the plain row, no card, outside the coordinator popover context", () => {
    renderRenderer("complete", { proposal_id: PROPOSAL_ID }, false);
    expect(screen.getByText(TITLE)).not.toBeNull();
    expect(screen.queryByTestId(`proposal-card-${PROPOSAL_ID}`)).toBeNull();
  });
});

describe("ProposeTaskRenderer - card states", () => {
  it("shows the loading placeholder before the proposal's first read resolves", () => {
    renderRenderer("running", { proposal_id: PROPOSAL_ID });
    expect(screen.getByText("Loading proposal")).not.toBeNull();
    expect(screen.queryByTestId(`proposal-card-${PROPOSAL_ID}`)).toBeNull();
  });

  it("shows the 404 copy with no card when the by-id read finds nothing", () => {
    useProposalByIdMock.mockReturnValue({ proposal: undefined, notFound: true });
    renderRenderer("complete", { proposal_id: PROPOSAL_ID });
    expect(screen.getByText("This proposal no longer exists.")).not.toBeNull();
    expect(screen.queryByTestId(`proposal-card-${PROPOSAL_ID}`)).toBeNull();
  });

  it("renders the compact ProposalCard once the proposal's own row has loaded", () => {
    const row = proposal();
    useProposalByIdMock.mockReturnValue({ proposal: row, notFound: false });
    useProposalWorkflowNamesMock.mockReturnValue({
      workflowNameById: new Map([["wf-1", "Build"]]),
      stepNameByWorkflowStep: new Map([["wf-1:step-1", "Review"]]),
    });

    renderRenderer("complete", { proposal_id: PROPOSAL_ID });

    expect(screen.getByTestId(`proposal-card-${PROPOSAL_ID}`)).not.toBeNull();
    expect(proposalCardCalls[0]).toMatchObject({
      variant: "compact",
      proposal: row,
      canManage: true,
      workspaceId: WORKSPACE_ID,
      coordinatorId: COORDINATOR_ID,
    });
    expect(useProposalByIdMock).toHaveBeenCalledWith(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID);
    expect(useProposalWorkflowNamesMock).toHaveBeenCalledWith("wf-1");
  });

  it("navigates to the Needs-you deep link and closes the popover from onNavigateToForm", () => {
    const row = proposal();
    useProposalByIdMock.mockReturnValue({ proposal: row, notFound: false });
    renderRenderer("complete", { proposal_id: PROPOSAL_ID });

    const onNavigateToForm = proposalCardCalls[0].onNavigateToForm as (
      form: "edit" | "reject",
    ) => void;
    onNavigateToForm("edit");

    expect(pushMock).toHaveBeenCalledWith(
      `/workspaces/${WORKSPACE_ID}/coordinator/${COORDINATOR_ID}?proposal=${PROPOSAL_ID}&form=edit`,
    );
    expect(closePopoverMock).toHaveBeenCalledTimes(1);
  });
});

describe("propose_resume, propose_message and propose_move renderers", () => {
  const cases = [
    ["resume", ProposeResumeRenderer, "Kandev: Propose Resume"],
    ["message", ProposeMessageRenderer, "Kandev: Propose Message"],
    ["move", ProposeMoveRenderer, "Kandev: Propose Move"],
  ] as const;

  for (const [kind, Renderer, title] of cases) {
    it(`${kind}: attaches the compact card by proposal_id under its own title`, () => {
      const row = { ...proposal(), kind } as unknown as Proposal;
      useProposalByIdMock.mockReturnValue({ proposal: row, notFound: false });
      render(
        <CoordinatorProposalProvider
          value={{
            workspaceId: WORKSPACE_ID,
            coordinatorId: COORDINATOR_ID,
            closePopover: closePopoverMock,
          }}
        >
          <Renderer args={undefined} result={{ proposal_id: PROPOSAL_ID }} status="complete" />
        </CoordinatorProposalProvider>,
      );
      expect(screen.getByText(title)).not.toBeNull();
      expect(screen.getByTestId(`proposal-card-${PROPOSAL_ID}`)).not.toBeNull();
      expect(proposalCardCalls[0].variant).toBe("compact");
    });

    it(`${kind}: renders the plain row with no card when the call errored`, () => {
      render(<Renderer args={undefined} result={{ proposal_id: PROPOSAL_ID }} status="error" />);
      expect(screen.getByText(title)).not.toBeNull();
      expect(screen.queryByTestId(`proposal-card-${PROPOSAL_ID}`)).toBeNull();
    });
  }
});

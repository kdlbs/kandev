import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ToastProvider } from "@/components/toast-provider";
import { useProposalsStore } from "@/hooks/domains/coordinator/use-proposals";
import type { Coordinator, Proposal } from "@/lib/api/domains/coordinator-api";
import type {
  ClassifyResult,
  NeedsYouItem,
  NeedsYouProposalItem,
} from "@/lib/coordinator/attention";
import type {
  CoordinatorReadyContext,
  CoordinatorRouteContentProps,
} from "./coordinator-route-content";
import { needsYouItemHeadingId } from "./components/needs-you-item-card";
import { NEEDS_YOU_EMPTY_HEADING_ID } from "./use-needs-you-focus";

let readyContext: CoordinatorReadyContext;
let capturedProps: CoordinatorRouteContentProps | undefined;

vi.mock("@/components/page-shell", () => ({
  PageShell: ({ title, children }: { title: string; children: React.ReactNode }) => (
    <div data-testid="stub-page-shell" data-title={title}>
      {children}
    </div>
  ),
}));

vi.mock("./coordinator-route-content", () => ({
  CoordinatorRouteContent: (props: CoordinatorRouteContentProps) => {
    capturedProps = props;
    return <>{props.children(readyContext)}</>;
  },
}));

let phase2On = false;
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => phase2On }));
vi.mock("./components/goal-note", () => ({
  GoalNote: (props: { coordinatorId: string; canManage: boolean }) => (
    <div data-testid="goal-note-stub" data-manage={String(props.canManage)}>
      {props.coordinatorId}
    </div>
  ),
}));

vi.mock("@/hooks/domains/coordinator/use-standing-orders", () => ({
  useStandingOrders: () => ({ orders: [], status: "ready", reload: vi.fn() }),
}));

import { NeedsYouPageClient } from "./needs-you-page-client";

const TIMESTAMP = "2026-09-27T00:00:00Z";

afterEach(() => {
  cleanup();
  phase2On = false;
});

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "co-1",
    workspace_id: "ws-1",
    name: "Planner",
    agent_profile_id: "a-1",
    executor_profile_id: "e-1",
    task_agent_profile_id: "ap-1",
    task_executor_profile_id: "ep-1",
    context: "",
    conversation_task_id: null,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

function emptyClassification(): ClassifyResult {
  return {
    needsYou: [],
    queue: { ready_to_merge: [], in_review: [], working: [], done: [], other: [] },
  };
}

function readyContextWith(needsYou: NeedsYouItem[], canManage = false): CoordinatorReadyContext {
  return {
    coordinator: coordinator(),
    coordinators: [coordinator()],
    canManage,
    attention: {
      classification: { ...emptyClassification(), needsYou },
      stepNameByTaskId: new Map(),
      workflowNameById: new Map(),
      stepNameByWorkflowStep: new Map(),
      openTasksById: new Map(),
      prsByTaskId: new Map(),
      watchSetUnavailable: false,
      watchSet: undefined,
      tasks: [],
      loadedAt: 1,
      error: false,
      tasksNeverLoaded: false,
      inputs: [
        { kind: "tasks", error: false, loadedAt: 1 },
        { kind: "stalls", error: false, loadedAt: 1 },
        { kind: "proposals", error: false, loadedAt: 1 },
      ],
      autonomy: { value: null, loadedAt: null, error: false, loading: false, retry: vi.fn() },
      phase3Effective: false,
      retryFailed: vi.fn(),
      computeNeedsYouCount: vi.fn(() => needsYou.length),
    },
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
        description: "desc",
        rationale: "rationale",
        workflow_id: "wf-1",
        step_id: "step-1",
        repository_id: "repo-1",
        source_task_id: "t-4",
      },
    },
    ...overrides,
  };
}

function proposalRow(overrides: Partial<Proposal> = {}): Proposal {
  return {
    id: "p-1",
    coordinator_id: "co-1",
    workspace_id: "ws-1",
    status: "pending",
    spec: {
      title: "Add tests",
      description: "desc",
      rationale: "rationale",
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

function setLocation(path: string) {
  window.history.replaceState({}, "", path);
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

beforeEach(() => {
  capturedProps = undefined;
  readyContext = readyContextWith([]);
  setLocation("/workspaces/ws-1/coordinator/co-1");
});

afterEach(() => {
  useProposalsStore.setState({ byCoordinator: {} });
  setLocation("/workspaces/ws-1/coordinator/co-1");
});

function renderPage() {
  return render(
    <ToastProvider>
      <TooltipProvider>
        <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
      </TooltipProvider>
    </ToastProvider>,
  );
}

describe("NeedsYouPageClient", () => {
  it("passes the needs-you view to the shared route content, which owns the page chrome", () => {
    renderPage();
    expect(capturedProps?.view).toBe("needs-you");
    expect(capturedProps?.workspaceId).toBe("ws-1");
    expect(capturedProps?.coordinatorId).toBe("co-1");
  });

  it("shows the empty state when there is nothing needing attention", () => {
    renderPage();
    expect(screen.getByTestId("empty-needs-you-state")).not.toBeNull();
    expect(screen.queryByTestId("needs-you-item-list")).toBeNull();
  });

  it("renders one item card per needs-you item", () => {
    readyContext = readyContextWith([
      {
        id: "task-1",
        kind: "question",
        referenceTimeMs: 1,
        ageMs: 1_000,
        task: { id: "task-1", title: "Task one" },
        pendingAction: "clarification",
      },
    ]);
    renderPage();
    expect(screen.getByTestId("needs-you-item-list")).not.toBeNull();
    expect(screen.getByTestId("needs-you-item-task-1")).not.toBeNull();
    expect(screen.queryByTestId("empty-needs-you-state")).toBeNull();
  });
});

describe("NeedsYouPageClient - deep-linked proposal form", () => {
  it("opens the reject form with focus on Reason, and clears the query params", async () => {
    setLocation("/workspaces/ws-1/coordinator/co-1?proposal=p-1&form=reject");
    seedProposal("co-1", proposalRow());
    readyContext = readyContextWith([proposalItem()], true);

    renderPage();

    const reason = await screen.findByLabelText("Reason (optional)");
    expect(document.activeElement).toBe(reason);
    await waitFor(() => expect(window.location.search).toBe(""));
  });

  it("shows a toast and clears the query when the deep-linked id is not a current item", async () => {
    setLocation("/workspaces/ws-1/coordinator/co-1?proposal=missing&form=edit");
    readyContext = readyContextWith([], true);

    renderPage();

    expect(
      await screen.findByText("This proposal is no longer waiting for a decision."),
    ).not.toBeNull();
    await waitFor(() => expect(window.location.search).toBe(""));
  });

  it("ignores an unrecognized form value and leaves the query untouched", () => {
    setLocation("/workspaces/ws-1/coordinator/co-1?proposal=p-1&form=bogus");
    seedProposal("co-1", proposalRow());
    readyContext = readyContextWith([proposalItem()], true);

    renderPage();

    expect(screen.queryByLabelText("Reason (optional)")).toBeNull();
    expect(window.location.search).toBe("?proposal=p-1&form=bogus");
  });
});

describe("NeedsYouPageClient - focus after a decision", () => {
  function questionItem(id: string): NeedsYouItem {
    return {
      kind: "question",
      id,
      referenceTimeMs: 0,
      ageMs: 1_000,
      task: { id, title: `Task ${id}` },
      pendingAction: "clarification",
    };
  }

  it("moves focus to the next item's heading when the focused item leaves the list, and shows no toast", () => {
    readyContext = readyContextWith([questionItem("t-1"), questionItem("t-2")]);
    const { rerender } = renderPage();

    document.getElementById(needsYouItemHeadingId("t-1"))?.focus();
    expect(document.activeElement?.id).toBe(needsYouItemHeadingId("t-1"));

    readyContext = readyContextWith([questionItem("t-2")]);
    rerender(
      <ToastProvider>
        <TooltipProvider>
          <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
        </TooltipProvider>
      </ToastProvider>,
    );

    expect(document.activeElement?.id).toBe(needsYouItemHeadingId("t-2"));
    expect(screen.queryByTestId("toast-message")).toBeNull();
  });

  it("moves focus to the previous item's heading when the last, focused item leaves the list", () => {
    readyContext = readyContextWith([questionItem("t-1"), questionItem("t-2")]);
    const { rerender } = renderPage();

    document.getElementById(needsYouItemHeadingId("t-2"))?.focus();

    readyContext = readyContextWith([questionItem("t-1")]);
    rerender(
      <ToastProvider>
        <TooltipProvider>
          <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
        </TooltipProvider>
      </ToastProvider>,
    );

    expect(document.activeElement?.id).toBe(needsYouItemHeadingId("t-1"));
  });

  it("moves focus to the empty state's heading when the only, focused item leaves the list", () => {
    readyContext = readyContextWith([questionItem("t-1")]);
    const { rerender } = renderPage();

    document.getElementById(needsYouItemHeadingId("t-1"))?.focus();

    readyContext = readyContextWith([]);
    rerender(
      <ToastProvider>
        <TooltipProvider>
          <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
        </TooltipProvider>
      </ToastProvider>,
    );

    expect(document.activeElement?.id).toBe(NEEDS_YOU_EMPTY_HEADING_ID);
  });

  it("does not move focus when the removed item did not have it", () => {
    readyContext = readyContextWith([questionItem("t-1"), questionItem("t-2")]);
    const { rerender } = renderPage();

    document.getElementById(needsYouItemHeadingId("t-2"))?.focus();

    readyContext = readyContextWith([questionItem("t-2")]);
    rerender(
      <ToastProvider>
        <TooltipProvider>
          <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
        </TooltipProvider>
      </ToastProvider>,
    );

    expect(document.activeElement?.id).toBe(needsYouItemHeadingId("t-2"));
  });
});

describe("NeedsYouPageClient: goal note", () => {
  it("renders no goal note while the phase-2 flag is off", () => {
    readyContext = readyContextWith([]);
    render(
      <ToastProvider>
        <TooltipProvider>
          <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
        </TooltipProvider>
      </ToastProvider>,
    );
    expect(screen.queryByTestId("goal-note-stub")).toBeNull();
  });

  it("renders the goal note above the body, empty state included, when the flag is on", () => {
    phase2On = true;
    readyContext = readyContextWith([], true);
    render(
      <ToastProvider>
        <TooltipProvider>
          <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
        </TooltipProvider>
      </ToastProvider>,
    );
    const note = screen.getByTestId("goal-note-stub");
    expect(note.textContent).toBe("co-1");
    expect(note.dataset.manage).toBe("true");
    expect(document.getElementById(NEEDS_YOU_EMPTY_HEADING_ID)).toBeTruthy();
  });
});

describe("NeedsYouPageClient: stall actions", () => {
  function stallItem(): NeedsYouItem {
    return {
      kind: "stall",
      id: "t-9",
      task: {
        id: "t-9",
        title: "Stuck task",
        identifier: "KAN-9",
        statusSummary: { primary_session: { id: "s-9", state: "FAILED" } },
      },
      stall: {
        task_id: "t-9",
        stalled_for_ms: 1,
        last_event_at: TIMESTAMP,
        detected_at: TIMESTAMP,
      },
      referenceTimeMs: 0,
      ageMs: 0,
    } as NeedsYouItem;
  }

  function renderStall(canManage: boolean) {
    readyContext = readyContextWith([stallItem()], canManage);
    render(
      <ToastProvider>
        <TooltipProvider>
          <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
        </TooltipProvider>
      </ToastProvider>,
    );
  }

  it("shows Resume, Open task and Show the evidence on a stall card with the flag on for a manager", () => {
    phase2On = true;
    renderStall(true);
    expect(screen.getByRole("button", { name: /^Resume/ })).not.toBeNull();
    expect(screen.getByText("Open task")).not.toBeNull();
    expect(screen.getByText("Show the evidence")).not.toBeNull();
  });

  it("shows no Resume with the flag off or for a reader", () => {
    renderStall(true);
    expect(screen.queryByRole("button", { name: /^Resume/ })).toBeNull();
    cleanup();
    phase2On = true;
    renderStall(false);
    expect(screen.queryByRole("button", { name: /^Resume/ })).toBeNull();
  });
});

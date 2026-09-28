import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { ClassifyResult } from "@/lib/coordinator/attention";
import type {
  CoordinatorInputStatus,
  UseCoordinatorAttentionResult,
} from "./use-coordinator-attention";
import type { ResolvedCoordinatorState } from "./use-resolved-coordinator";

const replaceMock = vi.fn();
const retryListMock = vi.fn();
const retryFailedMock = vi.fn();
const coordinatorCopilotCalls = vi.hoisted(() => [] as Array<Record<string, unknown>>);

let resolvedState: ResolvedCoordinatorState;
let attentionResult: UseCoordinatorAttentionResult;
let workspaceItems: Array<{ id: string; scopes: string[] | undefined }>;
let activeWorkspaceId: string;

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: replaceMock }),
}));

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) =>
    selector({
      workspaces: { items: workspaceItems, activeId: activeWorkspaceId },
    }),
}));

vi.mock("./use-resolved-coordinator", () => ({
  useResolvedCoordinator: () => resolvedState,
}));

vi.mock("./use-coordinator-attention", () => ({
  useCoordinatorAttention: () => attentionResult,
}));

vi.mock("./copilot/coordinator-copilot", () => ({
  CoordinatorCopilot: (props: Record<string, unknown>) => {
    coordinatorCopilotCalls.push(props);
    return <div data-testid="coordinator-copilot-marker" />;
  },
}));

import { CoordinatorRouteContent } from "./coordinator-route-content";

afterEach(cleanup);

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "co-1",
    workspace_id: "ws-1",
    name: "Planner",
    agent_profile_id: "a-1",
    executor_profile_id: "e-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

function emptyClassification(): ClassifyResult {
  return {
    needsYou: [],
    queue: { ready_to_merge: [], in_review: [], working: [], done: [], other: [] },
  };
}

function attention(
  overrides: Partial<UseCoordinatorAttentionResult> = {},
): UseCoordinatorAttentionResult {
  const inputs: CoordinatorInputStatus[] = [
    { kind: "tasks", error: false, loadedAt: 1 },
    { kind: "stalls", error: false, loadedAt: 1 },
    { kind: "proposals", error: false, loadedAt: 1 },
  ];
  return {
    classification: emptyClassification(),
    stepNameByTaskId: new Map(),
    workflowNameById: new Map(),
    stepNameByWorkflowStep: new Map(),
    openTasksById: new Map(),
    prsByTaskId: new Map(),
    tasksNeverLoaded: false,
    inputs,
    retryFailed: retryFailedMock,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  coordinatorCopilotCalls.length = 0;
  workspaceItems = [{ id: "ws-1", scopes: [] }];
  activeWorkspaceId = "ws-1";
  attentionResult = attention();
});

describe("CoordinatorRouteContent", () => {
  it("shows a loading indicator while the coordinator list is unresolved", () => {
    resolvedState = { status: "loading" };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId={null} view="needs-you">
        {() => <div data-testid="ready-content" />}
      </CoordinatorRouteContent>,
    );
    expect(screen.getByRole("status")).not.toBeNull();
    expect(screen.queryByTestId("ready-content")).toBeNull();
  });

  it("shows the list-error state and retries the list on Try again", () => {
    resolvedState = { status: "list-error", retry: retryListMock };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId={null} view="needs-you">
        {() => <div data-testid="ready-content" />}
      </CoordinatorRouteContent>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retryListMock).toHaveBeenCalledOnce();
  });

  it("shows the no-coordinator state, gated by scope for Add a coordinator", () => {
    resolvedState = { status: "no-coordinator" };
    workspaceItems = [{ id: "ws-1", scopes: ["workspace.manage"] }];
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId={null} view="needs-you">
        {() => <div data-testid="ready-content" />}
      </CoordinatorRouteContent>,
    );
    expect(screen.getByRole("link", { name: "Add a coordinator" })).not.toBeNull();
  });

  it("shows the unknown-coordinator state", () => {
    resolvedState = { status: "unknown-coordinator", retry: retryListMock };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId="co-x" view="needs-you">
        {() => <div data-testid="ready-content" />}
      </CoordinatorRouteContent>,
    );
    expect(screen.getByText("This coordinator is not in this workspace.")).not.toBeNull();
  });

  it("redirects to the first coordinator, on the requested view's route", () => {
    resolvedState = { status: "redirect", target: coordinator({ id: "co-2" }) };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId={null} view="queue">
        {() => <div data-testid="ready-content" />}
      </CoordinatorRouteContent>,
    );
    expect(replaceMock).toHaveBeenCalledWith("/workspaces/ws-1/coordinator/co-2/queue");
  });

  it("renders the header, count strip and ready content when ready", () => {
    resolvedState = {
      status: "ready",
      coordinator: coordinator(),
      coordinators: [coordinator()],
    };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId="co-1" view="needs-you">
        {({ coordinator: c }) => <div data-testid="ready-content">{c.name}</div>}
      </CoordinatorRouteContent>,
    );
    expect(screen.getByRole("heading", { name: "Planner" })).not.toBeNull();
    expect(screen.getByTestId("coordinator-count-strip")).not.toBeNull();
    expect(screen.getByTestId("ready-content").textContent).toBe("Planner");
    expect(screen.getByTestId("coordinator-copilot-marker")).not.toBeNull();
    expect(coordinatorCopilotCalls[0]).toMatchObject({
      workspaceId: "ws-1",
      coordinatorId: "co-1",
      coordinatorName: "Planner",
      canManage: false,
    });
  });
});

describe("CoordinatorRouteContent - tasks never loaded", () => {
  it("mounts the copilot even when tasks failed to load", () => {
    attentionResult = attention({
      tasksNeverLoaded: true,
      inputs: [
        { kind: "tasks", error: true, loadedAt: undefined },
        { kind: "stalls", error: false, loadedAt: 1 },
        { kind: "proposals", error: false, loadedAt: 1 },
      ],
    });
    resolvedState = {
      status: "ready",
      coordinator: coordinator(),
      coordinators: [coordinator()],
    };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId="co-1" view="needs-you">
        {() => <div data-testid="ready-content" />}
      </CoordinatorRouteContent>,
    );
    expect(screen.getByTestId("coordinator-copilot-marker")).not.toBeNull();
  });

  it("replaces the lists and count strip with the banner when tasks never loaded and failed", () => {
    attentionResult = attention({
      tasksNeverLoaded: true,
      inputs: [
        { kind: "tasks", error: true, loadedAt: undefined },
        { kind: "stalls", error: false, loadedAt: 1 },
        { kind: "proposals", error: false, loadedAt: 1 },
      ],
    });
    resolvedState = {
      status: "ready",
      coordinator: coordinator(),
      coordinators: [coordinator()],
    };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId="co-1" view="needs-you">
        {() => <div data-testid="ready-content" />}
      </CoordinatorRouteContent>,
    );
    expect(screen.queryByTestId("coordinator-count-strip")).toBeNull();
    expect(screen.queryByTestId("ready-content")).toBeNull();
    expect(screen.getByText("Could not load this workspace's tasks.")).not.toBeNull();
  });
});

describe("CoordinatorRouteContent canManage", () => {
  it("derives canManage from the route's own workspace, not the globally active workspace", () => {
    // The active workspace (ws-2) has manage; the route's own workspace
    // (ws-1) does not. A user who switched their active workspace elsewhere
    // (or a stale WS-driven activeId) must not gain manage rights on a
    // coordinator screen for a workspace they don't manage.
    workspaceItems = [
      { id: "ws-1", scopes: [] },
      { id: "ws-2", scopes: ["workspace.manage"] },
    ];
    activeWorkspaceId = "ws-2";
    resolvedState = {
      status: "ready",
      coordinator: coordinator(),
      coordinators: [coordinator()],
    };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId="co-1" view="needs-you">
        {({ canManage }) => <div data-testid="can-manage">{String(canManage)}</div>}
      </CoordinatorRouteContent>,
    );
    expect(screen.getByTestId("can-manage").textContent).toBe("false");
  });

  it("grants canManage when the route's own workspace has the scope, even if it is not active", () => {
    workspaceItems = [
      { id: "ws-1", scopes: ["workspace.manage"] },
      { id: "ws-2", scopes: [] },
    ];
    activeWorkspaceId = "ws-2";
    resolvedState = {
      status: "ready",
      coordinator: coordinator(),
      coordinators: [coordinator()],
    };
    render(
      <CoordinatorRouteContent workspaceId="ws-1" coordinatorId="co-1" view="needs-you">
        {({ canManage }) => <div data-testid="can-manage">{String(canManage)}</div>}
      </CoordinatorRouteContent>,
    );
    expect(screen.getByTestId("can-manage").textContent).toBe("true");
  });
});

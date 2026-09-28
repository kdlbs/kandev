import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { ClassifyResult, NeedsYouItem } from "@/lib/coordinator/attention";
import type {
  CoordinatorReadyContext,
  CoordinatorRouteContentProps,
} from "./coordinator-route-content";

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

import { NeedsYouPageClient } from "./needs-you-page-client";

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

function readyContextWith(needsYou: NeedsYouItem[]): CoordinatorReadyContext {
  return {
    coordinator: coordinator(),
    coordinators: [coordinator()],
    canManage: false,
    attention: {
      classification: { ...emptyClassification(), needsYou },
      stepNameByTaskId: new Map(),
      workflowNameById: new Map(),
      stepNameByWorkflowStep: new Map(),
      openTasksById: new Map(),
      prsByTaskId: new Map(),
      tasksNeverLoaded: false,
      inputs: [
        { kind: "tasks", error: false, loadedAt: 1 },
        { kind: "stalls", error: false, loadedAt: 1 },
        { kind: "proposals", error: false, loadedAt: 1 },
      ],
      retryFailed: vi.fn(),
    },
  };
}

beforeEach(() => {
  capturedProps = undefined;
  readyContext = readyContextWith([]);
});

function renderPage() {
  return render(
    <TooltipProvider>
      <NeedsYouPageClient workspaceId="ws-1" coordinatorId="co-1" />
    </TooltipProvider>,
  );
}

describe("NeedsYouPageClient", () => {
  it("renders the Needs you page shell and passes the needs-you view", () => {
    renderPage();
    expect(screen.getByTestId("stub-page-shell").getAttribute("data-title")).toBe("Needs you");
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

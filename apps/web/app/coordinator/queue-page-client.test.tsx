import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { ClassifyResult, QueueItem } from "@/lib/coordinator/attention";
import type {
  CoordinatorReadyContext,
  CoordinatorRouteContentProps,
} from "./coordinator-route-content";

let readyContext: CoordinatorReadyContext;
let capturedProps: CoordinatorRouteContentProps | undefined;
let searchString = "";

vi.mock("@/lib/routing/client-router", () => ({
  useSearchParams: () => new URLSearchParams(searchString),
}));

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

import { QueuePageClient } from "./queue-page-client";

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

function queueItem(id: string): QueueItem {
  return {
    group: "working",
    id,
    task: { id, title: `Task ${id}` },
    lastActivityAtMs: 1,
    ageMs: 1_000,
  };
}

function emptyClassification(): ClassifyResult {
  return {
    needsYou: [],
    queue: { ready_to_merge: [], in_review: [], working: [], done: [], other: [] },
  };
}

function readyContextWith(classification: ClassifyResult): CoordinatorReadyContext {
  return {
    coordinator: coordinator(),
    coordinators: [coordinator()],
    canManage: false,
    attention: {
      classification,
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
      computeNeedsYouCount: vi.fn(() => 0),
    },
  };
}

beforeEach(() => {
  capturedProps = undefined;
  searchString = "";
  readyContext = readyContextWith(emptyClassification());
});

describe("QueuePageClient", () => {
  it("renders the Queue page shell and passes the queue view", () => {
    render(<QueuePageClient workspaceId="ws-1" coordinatorId="co-1" />);
    expect(screen.getByTestId("stub-page-shell").getAttribute("data-title")).toBe("Queue");
    expect(capturedProps?.view).toBe("queue");
  });

  it("renders groups in Working, In review, Ready to merge, Done, Other order", () => {
    render(<QueuePageClient workspaceId="ws-1" coordinatorId="co-1" />);
    const list = screen.getByTestId("queue-group-list");
    const order = ["working", "in_review", "ready_to_merge", "done", "other"].map((group) =>
      list.querySelector(`[data-testid="queue-group-${group}"]`),
    );
    const rendered = Array.from(list.children);
    expect(rendered).toEqual(order);
  });

  it("shows each group's count from the classification", () => {
    readyContext = readyContextWith({
      ...emptyClassification(),
      queue: {
        ...emptyClassification().queue,
        working: [queueItem("t-1"), queueItem("t-2")],
      },
    });
    render(<QueuePageClient workspaceId="ws-1" coordinatorId="co-1" />);
    expect(screen.getByTestId("queue-group-working").textContent).toContain("2");
  });

  it("force-opens a collapsed-by-default group named by ?group=", () => {
    searchString = "group=done";
    render(<QueuePageClient workspaceId="ws-1" coordinatorId="co-1" />);
    const trigger = screen
      .getByTestId("queue-group-done")
      .querySelector("[aria-expanded]") as HTMLElement;
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
  });

  it("leaves other collapsed-by-default groups closed", () => {
    searchString = "group=done";
    render(<QueuePageClient workspaceId="ws-1" coordinatorId="co-1" />);
    const trigger = screen
      .getByTestId("queue-group-other")
      .querySelector("[aria-expanded]") as HTMLElement;
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });
});

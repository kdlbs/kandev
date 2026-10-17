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
let phase2Enabled = true;

vi.mock("@/lib/routing/client-router", () => ({
  useSearchParams: () => new URLSearchParams(searchString),
}));

vi.mock("./coordinator-route-content", () => ({
  CoordinatorRouteContent: (props: CoordinatorRouteContentProps) => {
    capturedProps = props;
    return <>{props.children(readyContext)}</>;
  },
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => phase2Enabled,
}));

vi.mock("./queue/what-it-did", () => ({
  WhatItDid: (props: { coordinatorId: string }) => (
    <div data-testid="what-it-did" data-coordinator={props.coordinatorId} />
  ),
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
    task_agent_profile_id: "ap-1",
    task_executor_profile_id: "ep-1",
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
      computeNeedsYouCount: vi.fn(() => 0),
    },
  };
}

beforeEach(() => {
  capturedProps = undefined;
  searchString = "";
  phase2Enabled = true;
  readyContext = readyContextWith(emptyClassification());
});

describe("QueuePageClient", () => {
  it("passes the queue view to the shared route content, which owns the page chrome", () => {
    render(<QueuePageClient workspaceId="ws-1" coordinatorId="co-1" />);
    expect(capturedProps?.view).toBe("queue");
  });

  it("renders groups in Working, In review, Ready to merge, Done, Other order", () => {
    phase2Enabled = false;
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

  it("renders What it did after the groups when the phase 2 flag is on", () => {
    render(<QueuePageClient workspaceId="ws-1" coordinatorId="co-1" />);
    const list = screen.getByTestId("queue-group-list");
    expect(list.lastElementChild).toBe(screen.getByTestId("what-it-did"));
    expect(screen.getByTestId("what-it-did").getAttribute("data-coordinator")).toBe("co-1");
  });

  it("omits What it did when the flag is off", () => {
    phase2Enabled = false;
    render(<QueuePageClient workspaceId="ws-1" coordinatorId="co-1" />);
    expect(screen.queryByTestId("what-it-did")).toBeNull();
  });
});

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttentionTask, QueueItem } from "@/lib/coordinator/attention";

const mockUseAppStore = vi.fn();
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => mockUseAppStore(selector),
}));

import { QueueRow } from "./queue-row";

afterEach(() => {
  cleanup();
  mockUseAppStore.mockReset();
});

function task(overrides: Partial<AttentionTask> = {}): AttentionTask {
  return { id: "t-1", title: "Task 1", identifier: "KAN-1", ...overrides };
}

function item(overrides: Partial<QueueItem> = {}): QueueItem {
  return {
    group: "other",
    id: "t-1",
    task: task(),
    lastActivityAtMs: undefined,
    ageMs: 5 * 60_000,
    ...overrides,
  };
}

describe("QueueRow", () => {
  it("shows the identifier, step and age", () => {
    mockUseAppStore.mockReturnValue(null);
    render(<QueueRow item={item()} stepNameByTaskId={new Map([["t-1", "Build"]])} />);
    expect(screen.getByText("KAN-1")).not.toBeNull();
    expect(screen.getByText("Build")).not.toBeNull();
    expect(screen.getByText("5m")).not.toBeNull();
  });

  it("shows the agent state for a Working row", () => {
    mockUseAppStore.mockReturnValue(null);
    render(
      <QueueRow
        item={item({
          group: "working",
          task: task({ statusSummary: { primary_session: { id: "s-1", state: "RUNNING" } } }),
        })}
        stepNameByTaskId={new Map()}
      />,
    );
    expect(screen.getByText("Running")).not.toBeNull();
  });

  it("shows PR detail unavailable with no PR loaded for an In review row", () => {
    mockUseAppStore.mockReturnValue(null);
    render(
      <QueueRow
        item={item({
          group: "in_review",
          task: task({ statusSummary: { pull_request: { state: "open" } } }),
        })}
        stepNameByTaskId={new Map()}
      />,
    );
    expect(screen.getByText("open")).not.toBeNull();
    expect(screen.getByText("PR detail unavailable")).not.toBeNull();
  });

  it("shows PR state, unresolved threads and checks state when the PR is loaded", () => {
    mockUseAppStore.mockReturnValue({
      state: "open",
      unresolved_review_threads: 2,
      checks_state: "pending",
    });
    render(<QueueRow item={item({ group: "ready_to_merge" })} stepNameByTaskId={new Map()} />);
    expect(screen.getByText("open")).not.toBeNull();
    expect(screen.getByText("pending")).not.toBeNull();
  });

  it("shows the underivable text for an unreadable Other row", () => {
    mockUseAppStore.mockReturnValue(null);
    render(
      <QueueRow
        item={item({ group: "other", sessionUnreadable: true })}
        stepNameByTaskId={new Map()}
      />,
    );
    expect(screen.getByText("position underivable: session unreadable")).not.toBeNull();
  });

  it("shows no session for an Other row with no primary session", () => {
    mockUseAppStore.mockReturnValue(null);
    render(
      <QueueRow
        item={item({
          group: "other",
          task: task({ statusSummary: { primary_session: null } }),
        })}
        stepNameByTaskId={new Map()}
      />,
    );
    expect(screen.getByText("No session")).not.toBeNull();
  });

  it("falls back to the task title with no identifier", () => {
    mockUseAppStore.mockReturnValue(null);
    render(
      <QueueRow
        item={item({ task: task({ identifier: undefined }) })}
        stepNameByTaskId={new Map()}
      />,
    );
    expect(screen.getByText("Task 1")).not.toBeNull();
  });
});

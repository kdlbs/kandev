import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FailedInboxRow as FailedInboxRowData } from "@/lib/types/failed-inbox";
import { FailedInboxTabPanel } from "./failed-inbox-tab-panel";

vi.mock("@/components/state-provider", async () => {
  const actual = await vi.importActual<typeof import("@/components/state-provider")>(
    "@/components/state-provider",
  );
  return { ...actual, useAppStore: (selector: (s: unknown) => unknown) => selector(mockState) };
});

let mockState: {
  workspaces: { activeId: string; items: Array<{ id: string; name: string }> };
  failedInbox: {
    byWorkspaceId: Record<
      string,
      { rows: FailedInboxRowData[]; count: number; truncated: boolean; status: string }
    >;
  };
};

function row(id: string): FailedInboxRowData {
  return {
    task_id: id,
    title: `Task ${id}`,
    workspace_id: "w1",
    origin: "manual",
    failure_instant: "2026-09-14T00:00:00Z",
    reason: "boom",
  };
}

beforeEach(() => {
  mockState = {
    workspaces: { activeId: "w1", items: [{ id: "w1", name: "Kegmil V2" }] },
    failedInbox: { byWorkspaceId: {} },
  };
});

afterEach(() => cleanup());

describe("FailedInboxTabPanel", () => {
  it("shows a loading indicator on the very first read", () => {
    mockState.failedInbox.byWorkspaceId.w1 = {
      rows: [],
      count: 0,
      truncated: false,
      status: "loading",
    };
    render(<FailedInboxTabPanel />);
    expect(screen.getByRole("status")).not.toBeNull();
  });

  it("renders the error state, not the empty state, when the read failed (AC .22)", () => {
    mockState.failedInbox.byWorkspaceId.w1 = {
      rows: [],
      count: 0,
      truncated: false,
      status: "error",
    };
    render(<FailedInboxTabPanel />);
    expect(screen.getByTestId("failed-inbox-error")).not.toBeNull();
    expect(screen.queryByTestId("failed-inbox-empty")).toBeNull();
  });

  it("renders the empty state when the read succeeded with no rows", () => {
    mockState.failedInbox.byWorkspaceId.w1 = {
      rows: [],
      count: 0,
      truncated: false,
      status: "ready",
    };
    render(<FailedInboxTabPanel />);
    expect(screen.getByTestId("failed-inbox-empty")).not.toBeNull();
  });

  it("renders a row per listed failed task", () => {
    mockState.failedInbox.byWorkspaceId.w1 = {
      rows: [row("t1"), row("t2")],
      count: 2,
      truncated: false,
      status: "ready",
    };
    render(<FailedInboxTabPanel />);
    expect(screen.getAllByTestId("failed-inbox-row")).toHaveLength(2);
  });

  it("shows a truncation notice when the page is bounded", () => {
    mockState.failedInbox.byWorkspaceId.w1 = {
      rows: [row("t1")],
      count: 1,
      truncated: true,
      status: "ready",
    };
    render(<FailedInboxTabPanel />);
    expect(screen.getByTestId("failed-inbox-truncated")).not.toBeNull();
  });
});

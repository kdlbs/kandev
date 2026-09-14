import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import type { FailedInboxRow as FailedInboxRowData } from "@/lib/types/failed-inbox";

const mocks = vi.hoisted(() => ({
  bumpRefreshTick: vi.fn(),
  useFailedInboxController: vi.fn(),
}));

const EMPTY_TESTID = "needs-you-inbox-empty";

let needsYouState: {
  status: "idle" | "loading" | "ready" | "error";
  bundles: ClarificationInboxBundle[];
  hiddenCount: number;
  hasMore: boolean;
};

let failedState: {
  rows: FailedInboxRowData[];
  count: number;
  truncated: boolean;
  status: "idle" | "loading" | "ready" | "error";
};

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) =>
    selector({
      needsYouInbox: { byWorkspaceId: { w1: needsYouState } },
      failedInbox: { byWorkspaceId: { w1: failedState } },
      workspaces: { activeId: "w1", items: [{ id: "w1", name: "Kegmil V2" }] },
      bumpNeedsYouInboxRefreshTick: mocks.bumpRefreshTick,
    }),
}));

// The shell is the app's own chrome (topbar, nav trigger, scroll container)
// and pulls the whole nav context in with it; these cases are about which view
// the page resolves to, so it is stubbed down to the title it is handed.
vi.mock("@/components/page-shell", () => ({
  PageShell: ({ title, children }: { title: string; children: React.ReactNode }) => (
    <div data-testid="stub-page-shell" data-title={title}>
      {children}
    </div>
  ),
}));

vi.mock("@/components/needs-you-inbox/needs-you-inbox-row", () => ({
  NeedsYouInboxRow: ({ bundle }: { bundle: ClarificationInboxBundle }) => (
    <div data-testid="stub-row">{bundle.pending_id}</div>
  ),
}));

vi.mock("@/components/needs-you-inbox/needs-you-inbox-hidden-panel", () => ({
  NeedsYouInboxHiddenPanel: ({ hiddenCount }: { hiddenCount: number }) => (
    <div data-testid="stub-hidden-panel">{hiddenCount}</div>
  ),
}));

// The controller owns its own refresh triggers (mount, tab change, workspace
// change, foreground, periodic 60s) and is covered by its own test suite;
// this page-level suite is about which view renders, so the controller is a
// no-op here.
vi.mock("@/hooks/domains/failed-inbox/use-failed-inbox-controller", () => ({
  useFailedInboxController: (...args: unknown[]) => mocks.useFailedInboxController(...args),
}));

import { NeedsYouInboxPageClient } from "./needs-you-inbox-page-client";

function bundle(id: string): ClarificationInboxBundle {
  return {
    pending_id: id,
    task_id: "t1",
    session_id: "s1",
    session_state: "WAITING_FOR_INPUT",
    task_title: "Task",
    created_at: "2026-09-03T04:54:02Z",
    context: "",
    messages: [
      {
        id: "m1",
        session_id: toSessionId("s1"),
        task_id: toTaskId("t1"),
        author_type: "agent",
        content: "",
        type: "clarification_request",
        created_at: "2026-09-03T04:54:02Z",
        metadata: {
          pending_id: id,
          session_id: "s1",
          question: { id: "q1", title: "Q", prompt: "", options: [] },
        },
      },
    ],
  };
}

function failedRow(id: string): FailedInboxRowData {
  return {
    task_id: id,
    title: `Failed task ${id}`,
    workspace_id: "w1",
    origin: "manual",
    failure_instant: "2026-09-14T00:00:00Z",
    reason: "boom",
  };
}

function setLocation(path: string) {
  window.history.replaceState({}, "", path);
}

beforeEach(() => {
  mocks.bumpRefreshTick.mockReset();
  mocks.useFailedInboxController.mockReset();
  needsYouState = { status: "idle", bundles: [], hiddenCount: 0, hasMore: false };
  failedState = { rows: [], count: 0, truncated: false, status: "idle" };
  setLocation("/needs-you-inbox");
});

afterEach(() => {
  cleanup();
  setLocation("/needs-you-inbox");
});

describe("NeedsYouInboxPageClient", () => {
  it("titles the route Inbox, which the app top bar owns (design-03#D4)", () => {
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("stub-page-shell").getAttribute("data-title")).toBe("Inbox");
  });

  it("renders a row per listed bundle", () => {
    needsYouState = {
      status: "ready",
      bundles: [bundle("p1"), bundle("p2")],
      hiddenCount: 0,
      hasMore: false,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getAllByTestId("stub-row")).toHaveLength(2);
  });

  it("renders the empty state when the read succeeded with no rows and no truncation", () => {
    needsYouState = { status: "ready", bundles: [], hiddenCount: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId(EMPTY_TESTID)).not.toBeNull();
  });

  it("renders the error state, not the empty state, when the read failed (AC .21)", () => {
    needsYouState = { status: "error", bundles: [], hiddenCount: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("needs-you-inbox-error")).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
  });

  it("renders the error state for a zero-row truncated page rather than caught up (F42)", () => {
    needsYouState = { status: "ready", bundles: [], hiddenCount: 0, hasMore: true };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("needs-you-inbox-error")).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
  });

  it("shows the truncation notice when the page is bounded (AC .11)", () => {
    needsYouState = { status: "ready", bundles: [bundle("p1")], hiddenCount: 0, hasMore: true };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("needs-you-inbox-truncated")).not.toBeNull();
  });

  it("does not show the truncation notice when the page is exhaustive", () => {
    needsYouState = { status: "ready", bundles: [bundle("p1")], hiddenCount: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.queryByTestId("needs-you-inbox-truncated")).toBeNull();
  });

  it("shows the hidden panel alongside a non-empty list when bundles are hidden", () => {
    needsYouState = { status: "ready", bundles: [bundle("p1")], hiddenCount: 2, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("stub-hidden-panel").textContent).toBe("2");
  });

  it("shows a loading indicator on the very first read, not the empty state", () => {
    needsYouState = { status: "loading", bundles: [], hiddenCount: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByRole("status")).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
  });
});

describe("NeedsYouInboxPageClient — Failed tab (REQ-UI-INBOX-FAILED-001)", () => {
  it("renders the tab strip with Needs you selected by default", () => {
    render(<NeedsYouInboxPageClient />);
    expect(screen.getByRole("tab", { name: /Needs you/ }).getAttribute("aria-selected")).toBe(
      "true",
    );
  });

  it("renders the Failed tab directly from ?tab=failed, without first rendering Needs you (AC .2)", () => {
    setLocation("/needs-you-inbox?tab=failed");
    needsYouState = { status: "ready", bundles: [bundle("p1")], hiddenCount: 0, hasMore: false };
    failedState = { rows: [failedRow("t1")], count: 1, truncated: false, status: "ready" };

    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("failed-inbox-row")).not.toBeNull();
    expect(screen.queryByTestId("stub-row")).toBeNull();
  });

  it("renders Needs you for an unrecognised tab value rather than an error (AC .2)", () => {
    setLocation("/needs-you-inbox?tab=bogus");
    needsYouState = { status: "ready", bundles: [bundle("p1")], hiddenCount: 0, hasMore: false };

    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("stub-row")).not.toBeNull();
  });

  it("shows the Failed tab's own badge count once known, even while Needs you is selected (AC .16)", () => {
    failedState = { rows: [failedRow("t1")], count: 1, truncated: false, status: "ready" };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("inbox-tab-failed-badge").textContent).toBe("1");
  });

  it("passes the failed count into the Needs-you empty state's AC .23 clause", () => {
    needsYouState = { status: "ready", bundles: [], hiddenCount: 0, hasMore: false };
    failedState = { rows: [failedRow("t1")], count: 1, truncated: false, status: "ready" };

    render(<NeedsYouInboxPageClient />);

    expect(screen.getByText(/also failed/)).not.toBeNull();
  });

  it("mounts the failed-inbox controller with the currently selected tab", () => {
    setLocation("/needs-you-inbox?tab=failed");
    render(<NeedsYouInboxPageClient />);

    expect(mocks.useFailedInboxController).toHaveBeenCalledWith("failed");
  });
});

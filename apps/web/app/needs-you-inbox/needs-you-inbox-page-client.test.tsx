import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";
import type { FailedInboxRow as FailedInboxRowData } from "@/lib/types/failed-inbox";

const mocks = vi.hoisted(() => ({
  bumpRefreshTick: vi.fn(),
  useFailedInboxController: vi.fn(),
  inboxHistoryRefresh: vi.fn(),
}));

const EMPTY_TESTID = "needs-you-inbox-empty";
const ERROR_TESTID = "needs-you-inbox-error";
const HISTORY_TAB_TESTID = "inbox-tab-history";

let needsYouState: {
  status: "idle" | "loading" | "ready" | "error";
  bundles: ClarificationInboxBundle[];
  hiddenCount: number;
  hasMore: boolean;
  appliedGeneration?: number;
  lastAppliedOk?: boolean;
};
let workspacesActiveId: string | null;

let failedState: {
  rows: FailedInboxRowData[];
  count: number;
  truncated: boolean;
  status: "idle" | "loading" | "ready" | "error";
};

let historyState: {
  status: "idle" | "loading" | "ready" | "error";
  bundles: unknown[];
  total: number;
  hasMore: boolean;
};

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) =>
    selector({
      needsYouInbox: { byWorkspaceId: { w1: needsYouState } },
      failedInbox: { byWorkspaceId: { w1: failedState } },
      inboxHistory: { byWorkspaceId: { w1: historyState } },
      workspaces: { activeId: workspacesActiveId, items: [{ id: "w1", name: "Kegmil V2" }] },
      bumpNeedsYouInboxRefreshTick: mocks.bumpRefreshTick,
    }),
}));

vi.mock("@/hooks/domains/inbox-history/use-inbox-history-controller", () => ({
  useInboxHistoryController: () => mocks.inboxHistoryRefresh,
}));

vi.mock("@/components/inbox-history/inbox-history-list", () => ({
  InboxHistoryList: ({ bundles }: { bundles: unknown[] }) => (
    <div data-testid="stub-history-list">{bundles.length}</div>
  ),
}));

vi.mock("@/components/inbox-history/inbox-history-empty-state", () => ({
  InboxHistoryEmptyState: () => <div data-testid="inbox-history-empty" />,
}));

vi.mock("@/components/inbox-history/inbox-history-error-state", () => ({
  InboxHistoryErrorState: ({ onRetry }: { onRetry: () => void }) => (
    <button type="button" data-testid="inbox-history-error" onClick={onRetry}>
      retry
    </button>
  ),
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
  mocks.inboxHistoryRefresh.mockReset();
  needsYouState = { status: "idle", bundles: [], hiddenCount: 0, hasMore: false };
  failedState = { rows: [], count: 0, truncated: false, status: "idle" };
  historyState = { status: "idle", bundles: [], total: 0, hasMore: false };
  workspacesActiveId = "w1";
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

    expect(screen.getByTestId(ERROR_TESTID)).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
  });

  it("renders the error state for a zero-row truncated page rather than caught up (F42)", () => {
    needsYouState = { status: "ready", bundles: [], hiddenCount: 0, hasMore: true };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId(ERROR_TESTID)).not.toBeNull();
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
});

describe("NeedsYouInboxPageClient loading vs. settled/refresh states", () => {
  it("shows a loading indicator on the very first read, not the empty state", () => {
    needsYouState = {
      status: "loading",
      bundles: [],
      hiddenCount: 0,
      hasMore: false,
      appliedGeneration: 0,
      lastAppliedOk: false,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByRole("status")).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
  });

  it("keeps the settled empty state during a background refresh, instead of re-flashing loading", () => {
    needsYouState = {
      status: "loading",
      bundles: [],
      hiddenCount: 0,
      hasMore: false,
      appliedGeneration: 1,
      lastAppliedOk: true,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId(EMPTY_TESTID)).not.toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("keeps the settled list during a background refresh, instead of re-flashing loading", () => {
    needsYouState = {
      status: "loading",
      bundles: [bundle("p1")],
      hiddenCount: 0,
      hasMore: false,
      appliedGeneration: 1,
      lastAppliedOk: true,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getAllByTestId("stub-row")).toHaveLength(1);
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("does not flash the error state for a truncated boot payload before the first read resolves", () => {
    needsYouState = {
      status: "loading",
      bundles: [],
      hiddenCount: 0,
      hasMore: true,
      appliedGeneration: 0,
      lastAppliedOk: false,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByRole("status")).not.toBeNull();
    expect(screen.queryByTestId(ERROR_TESTID)).toBeNull();
  });

  // R2-F1: a refresh that follows a FAILED read reaches resolveViewMode as
  // the same (loading, 0 bundles, 0 hidden, hasMore=false) tuple as a refresh
  // over an already-settled empty inbox -- only `lastAppliedOk` (false here,
  // since the last applied response was the error, not a page) tells them
  // apart. Getting this wrong renders a false "Nothing needs your attention"
  // over a read that is still failing.
  it("shows the loading indicator, not a false empty state, during a refresh that follows a failed read", () => {
    needsYouState = {
      status: "loading",
      bundles: [],
      hiddenCount: 0,
      hasMore: false,
      appliedGeneration: 3,
      lastAppliedOk: false,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByRole("status")).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
    expect(screen.queryByTestId(ERROR_TESTID)).toBeNull();
  });

  // R2-F5: the boot seed applies before the first read starts, so `status`
  // can observably be "idle" (not yet "loading") with a truncation flag
  // already set. That must read the same as any other pre-first-read state:
  // the honest spinner, not a false empty.
  it("shows the loading indicator, not a false empty state, for an idle truncated boot seed", () => {
    needsYouState = {
      status: "idle",
      bundles: [],
      hiddenCount: 0,
      hasMore: true,
      appliedGeneration: 0,
      lastAppliedOk: false,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByRole("status")).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
    expect(screen.queryByTestId(ERROR_TESTID)).toBeNull();
  });

  // R3-F1: with no active workspace the controller never issues a read (every
  // trigger is guarded on `workspaceId`), so `lastAppliedOk` can never flip
  // and the pre-read "idle"/`!lastAppliedOk` spinner would otherwise never
  // resolve. This must render the empty state, not a permanent spinner.
  it("renders the empty state, not a permanent spinner, when no active workspace resolves", () => {
    workspacesActiveId = null;
    needsYouState = {
      status: "idle",
      bundles: [],
      hiddenCount: 0,
      hasMore: false,
      appliedGeneration: 0,
      lastAppliedOk: false,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId(EMPTY_TESTID)).not.toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
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

// AC .1: a variant="line" tab strip, "Needs you" first and default-selected,
// "History" second. AC .16: the History tab's own bundle count.
describe("NeedsYouInboxPageClient — tab strip", () => {
  it("selects Needs you by default, with its row set, count and behavior unchanged (AC .1)", () => {
    needsYouState = { status: "ready", bundles: [bundle("p1")], hiddenCount: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getAllByTestId("stub-row")).toHaveLength(1);
    expect(screen.queryByTestId("stub-history-list")).toBeNull();
  });

  it("renders History as the second tab and switches content on selection", () => {
    historyState = { status: "ready", bundles: [{ pending_id: "h1" }], total: 1, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    // Radix TabsTrigger switches tabs on mousedown, not click.
    fireEvent.mouseDown(screen.getByTestId(HISTORY_TAB_TESTID));

    expect(screen.getByTestId("stub-history-list").textContent).toBe("1");
  });

  it("shows the History count badge in the Badge variant=secondary idiom, omitted at zero (AC .16)", () => {
    historyState = { status: "ready", bundles: [], total: 0, hasMore: false };
    const { rerender } = render(<NeedsYouInboxPageClient />);
    expect(screen.getByTestId(HISTORY_TAB_TESTID).querySelector("[data-slot=badge]")).toBeNull();

    historyState = { status: "ready", bundles: [], total: 3, hasMore: false };
    rerender(<NeedsYouInboxPageClient />);
    const badge = screen.getByTestId(HISTORY_TAB_TESTID).querySelector("[data-slot=badge]");
    expect(badge).not.toBeNull();
    expect(badge?.textContent).toBe("3");
  });

  it("renders the History error state, not the empty state, when the history read failed (AC .25)", () => {
    historyState = { status: "error", bundles: [], total: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);
    fireEvent.mouseDown(screen.getByTestId(HISTORY_TAB_TESTID));

    expect(screen.getByTestId("inbox-history-error")).not.toBeNull();
    expect(screen.queryByTestId("inbox-history-empty")).toBeNull();
  });

  it("renders the History empty state naming the bucket when there is nothing to show (AC .24)", () => {
    historyState = { status: "ready", bundles: [], total: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);
    fireEvent.mouseDown(screen.getByTestId(HISTORY_TAB_TESTID));

    expect(screen.getByTestId("inbox-history-empty")).not.toBeNull();
  });
});

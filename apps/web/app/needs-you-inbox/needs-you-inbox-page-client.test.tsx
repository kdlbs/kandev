import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { ClarificationInboxBundle } from "@/lib/types/clarification-inbox";

const mocks = vi.hoisted(() => ({
  bumpRefreshTick: vi.fn(),
}));

const EMPTY_TESTID = "needs-you-inbox-empty";
const ERROR_TESTID = "needs-you-inbox-error";

let state: {
  status: "idle" | "loading" | "ready" | "error";
  bundles: ClarificationInboxBundle[];
  hiddenCount: number;
  hasMore: boolean;
  appliedGeneration?: number;
  lastAppliedOk?: boolean;
};

vi.mock("@/components/state-provider", () => ({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  useAppStore: (selector: (s: any) => unknown) =>
    selector({
      needsYouInbox: { byWorkspaceId: { w1: state } },
      workspaces: { activeId: "w1" },
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

beforeEach(() => {
  mocks.bumpRefreshTick.mockReset();
  state = { status: "idle", bundles: [], hiddenCount: 0, hasMore: false };
});

afterEach(() => cleanup());

describe("NeedsYouInboxPageClient", () => {
  it("titles the route Inbox, which the app top bar owns (design-03#D4)", () => {
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("stub-page-shell").getAttribute("data-title")).toBe("Inbox");
  });

  it("renders a row per listed bundle", () => {
    state = {
      status: "ready",
      bundles: [bundle("p1"), bundle("p2")],
      hiddenCount: 0,
      hasMore: false,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getAllByTestId("stub-row")).toHaveLength(2);
  });

  it("renders the empty state when the read succeeded with no rows and no truncation", () => {
    state = { status: "ready", bundles: [], hiddenCount: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId(EMPTY_TESTID)).not.toBeNull();
  });

  it("renders the error state, not the empty state, when the read failed (AC .21)", () => {
    state = { status: "error", bundles: [], hiddenCount: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId(ERROR_TESTID)).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
  });

  it("renders the error state for a zero-row truncated page rather than caught up (F42)", () => {
    state = { status: "ready", bundles: [], hiddenCount: 0, hasMore: true };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId(ERROR_TESTID)).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
  });

  it("shows the truncation notice when the page is bounded (AC .11)", () => {
    state = { status: "ready", bundles: [bundle("p1")], hiddenCount: 0, hasMore: true };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("needs-you-inbox-truncated")).not.toBeNull();
  });

  it("does not show the truncation notice when the page is exhaustive", () => {
    state = { status: "ready", bundles: [bundle("p1")], hiddenCount: 0, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.queryByTestId("needs-you-inbox-truncated")).toBeNull();
  });

  it("shows the hidden panel alongside a non-empty list when bundles are hidden", () => {
    state = { status: "ready", bundles: [bundle("p1")], hiddenCount: 2, hasMore: false };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByTestId("stub-hidden-panel").textContent).toBe("2");
  });
});

describe("NeedsYouInboxPageClient loading vs. settled/refresh states", () => {
  it("shows a loading indicator on the very first read, not the empty state", () => {
    state = {
      status: "loading",
      bundles: [],
      hiddenCount: 0,
      hasMore: false,
      appliedGeneration: 0,
    };
    render(<NeedsYouInboxPageClient />);

    expect(screen.getByRole("status")).not.toBeNull();
    expect(screen.queryByTestId(EMPTY_TESTID)).toBeNull();
  });

  it("keeps the settled empty state during a background refresh, instead of re-flashing loading", () => {
    state = {
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

  it("does not flash the error state for a truncated boot payload before the first read resolves", () => {
    state = {
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
    state = {
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
    state = {
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
});

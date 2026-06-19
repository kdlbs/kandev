import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { useCommentsStore, isPRFeedbackComment } from "@/lib/state/slices/comments";
import { PRMergeabilityRow, blockedReason } from "./pr-mergeability-row";
import type { AppState } from "@/lib/state/store";
import type { MergeableState, TaskPR } from "@/lib/types/github";

const SESSION_ID = "sess-1";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "task-1",
    owner: "o",
    repo: "r",
    pr_number: 7,
    pr_url: "",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

function renderRow(pr: TaskPR, sessionId: string | null = SESSION_ID) {
  const initialState = {
    tasks: { activeSessionId: sessionId },
  } as unknown as Partial<AppState>;
  return render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <div data-testid="row-host">
          <PRMergeabilityRow pr={pr} />
        </div>
      </ToastProvider>
    </StateProvider>,
  );
}

beforeEach(() => {
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
});
afterEach(() => cleanup());

describe("PRMergeabilityRow", () => {
  it("shows the conflict banner with a Resolve conflicts CTA for a dirty PR", () => {
    const { queryByTestId } = renderRow(makePR({ mergeable_state: "dirty" }));
    expect(queryByTestId("pr-conflict-banner")).not.toBeNull();
    expect(queryByTestId("pr-resolve-conflicts-button")).not.toBeNull();
  });

  it("explains *why* a blocked PR is gated (not just 'Blocked')", () => {
    const { container, queryByTestId } = renderRow(
      makePR({ mergeable_state: "blocked", checks_state: "failure" }),
    );
    expect(queryByTestId("pr-conflict-banner")).toBeNull();
    expect(queryByTestId("pr-blocked-note")).not.toBeNull();
    expect(container.textContent).toContain("Blocked by branch protection");
  });

  it("stays silent for a blocked PR that is only awaiting a requested review", () => {
    // The block is just an outstanding reviewer — the review row + calm chip
    // already convey that, so the row must not show a contradictory note/chip.
    const { getByTestId } = renderRow(
      makePR({
        mergeable_state: "blocked",
        review_state: "approved",
        checks_state: "success",
        pending_review_count: 1,
      }),
    );
    expect(getByTestId("row-host").childElementCount).toBe(0);
  });

  it("shows a Behind base chip for a behind PR", () => {
    const { container } = renderRow(makePR({ mergeable_state: "behind" }));
    expect(container.textContent).toContain("Behind base");
  });

  it.each(["clean", "unstable", "has_hooks", "unknown", ""] as MergeableState[])(
    "renders nothing for mergeable_state=%s (stays quiet, no false alarm)",
    (mergeable_state) => {
      const { getByTestId } = renderRow(makePR({ mergeable_state }));
      expect(getByTestId("row-host").childElementCount).toBe(0);
    },
  );

  it("renders nothing for a non-open PR even when dirty", () => {
    const { getByTestId } = renderRow(makePR({ state: "merged", mergeable_state: "dirty" }));
    expect(getByTestId("row-host").childElementCount).toBe(0);
  });

  it("queues a conflict-resolution prompt for the active session when the CTA is clicked", () => {
    const { getByTestId } = renderRow(makePR({ mergeable_state: "dirty", pr_number: 7 }));
    fireEvent.click(getByTestId("pr-resolve-conflicts-button"));

    const queued = useCommentsStore
      .getState()
      .pendingForChat.map((id) => useCommentsStore.getState().byId[id])
      .filter((c) => !!c && isPRFeedbackComment(c));
    expect(queued).toHaveLength(1);
    const comment = queued[0]!;
    expect(isPRFeedbackComment(comment) && comment.feedbackType).toBe("conflict");
    expect(isPRFeedbackComment(comment) && comment.prNumber).toBe(7);
    expect(isPRFeedbackComment(comment) && comment.sessionId).toBe(SESSION_ID);
    expect(comment.content.toLowerCase()).toContain("conflict");
  });

  it("hides the Resolve conflicts CTA when there is no active session", () => {
    const { queryByTestId } = renderRow(makePR({ mergeable_state: "dirty" }), null);
    // Banner still surfaces the conflict, but the CTA needs a session to target.
    expect(queryByTestId("pr-conflict-banner")).not.toBeNull();
    expect(queryByTestId("pr-resolve-conflicts-button")).toBeNull();
  });
});

describe("blockedReason", () => {
  it("names the approval shortfall when required reviews aren't met", () => {
    expect(blockedReason(makePR({ required_reviews: 2, review_count: 0 }))).toContain(
      "2 more approvals",
    );
    expect(blockedReason(makePR({ required_reviews: 2, review_count: 1 }))).toContain(
      "1 more approval",
    );
  });

  it("points at a failing required check when CI is failure/pending", () => {
    expect(blockedReason(makePR({ checks_state: "failure" })).toLowerCase()).toContain(
      "required status check",
    );
    expect(blockedReason(makePR({ checks_state: "pending" })).toLowerCase()).toContain(
      "required status check",
    );
  });

  it("does not claim a check failed when no CI is configured (empty checks_state)", () => {
    // Regression: `checks_state !== "success"` also matched "" and falsely
    // reported a status-check block on a code-owners/conversation gate.
    const msg = blockedReason(makePR({ checks_state: "" })).toLowerCase();
    expect(msg).not.toContain("required status check");
    expect(msg).toContain("branch protection rule");
  });

  it("falls back to a generic protection note for other rules", () => {
    const msg = blockedReason(
      makePR({ checks_state: "success", required_reviews: 1, review_count: 1 }),
    );
    expect(msg.toLowerCase()).toContain("branch protection rule");
  });
});

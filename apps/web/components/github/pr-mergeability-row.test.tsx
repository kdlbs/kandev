import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { useCommentsStore, isPRFeedbackComment } from "@/lib/state/slices/comments";
import { PRMergeabilityRow, blockedReason } from "./pr-mergeability-row";
import type { AppState } from "@/lib/state/store";
import type { MergeableState, TaskPR } from "@/lib/types/github";

const SESSION_ID = "sess-1";
const QUEUE_STATUS_TEST_ID = "pr-merge-queue-status";
const CONFLICT_BANNER_TEST_ID = "pr-conflict-banner";

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
    expect(queryByTestId(CONFLICT_BANNER_TEST_ID)).not.toBeNull();
    expect(queryByTestId("pr-resolve-conflicts-button")).not.toBeNull();
  });

  it("explains *why* a blocked PR is gated (not just 'Blocked')", () => {
    const { container, queryByTestId } = renderRow(
      makePR({ mergeable_state: "blocked", checks_state: "failure" }),
    );
    expect(queryByTestId(CONFLICT_BANNER_TEST_ID)).toBeNull();
    expect(queryByTestId("pr-blocked-note")).not.toBeNull();
    expect(queryByTestId("pr-blocked-shield-icon")).not.toBeNull();
    expect(container.textContent).toContain("Blocked by branch protection");
  });

  it("keeps an otherwise green blocked PR neutral until GitHub accepts it", () => {
    const { queryByTestId } = renderRow(
      makePR({
        mergeable_state: "blocked",
        checks_state: "success",
        review_state: "approved",
      }),
    );
    expect(queryByTestId("pr-branch-protection-wait-note")).not.toBeNull();
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
    expect(queryByTestId(CONFLICT_BANNER_TEST_ID)).not.toBeNull();
    expect(queryByTestId("pr-resolve-conflicts-button")).toBeNull();
  });
});

describe("PRMergeabilityRow queue state", () => {
  it("replaces mergeability with queue state and metadata for an active entry", () => {
    const { getByTestId, queryByTestId, container } = renderRow(
      makePR({
        merge_queue_state: "mergeable",
        merge_queue_position: 2,
        merge_queue_estimated_time_to_merge_seconds: 120,
        mergeable_state: "dirty",
      }),
    );

    expect(getByTestId(QUEUE_STATUS_TEST_ID)).not.toBeNull();
    expect(container.textContent).toContain("Merge queue: Mergeable");
    expect(container.textContent).toContain("Position 2");
    expect(container.textContent).toContain("2 minutes");
    expect(queryByTestId(CONFLICT_BANNER_TEST_ID)).toBeNull();
  });

  it("keeps queue state visible when position and estimate are absent", () => {
    const { getByTestId, container } = renderRow(
      makePR({ merge_queue_state: "locked", mergeable_state: "blocked" }),
    );

    expect(getByTestId(QUEUE_STATUS_TEST_ID)).not.toBeNull();
    expect(container.textContent).toContain("Merge queue: Locked");
    expect(container.textContent).not.toContain("Position");
  });

  it("does not show a stale queue entry after a terminal transition", () => {
    const { queryByTestId, getByTestId } = renderRow(
      makePR({ state: "merged", merge_queue_state: "queued" }),
    );

    expect(queryByTestId(QUEUE_STATUS_TEST_ID)).toBeNull();
    expect(getByTestId("row-host").childElementCount).toBe(0);
  });
});

describe("blockedReason", () => {
  it("names the approval shortfall when required reviews aren't met", () => {
    expect(blockedReason(makePR({ required_reviews: 2, review_count: 0 }), t)).toContain(
      "2 more approvals",
    );
    expect(blockedReason(makePR({ required_reviews: 2, review_count: 1 }), t)).toContain(
      "1 more approval",
    );
  });

  it("points at a failing required check when CI is failure/pending", () => {
    expect(blockedReason(makePR({ checks_state: "failure" }), t).toLowerCase()).toContain(
      "required status check",
    );
    expect(blockedReason(makePR({ checks_state: "pending" }), t).toLowerCase()).toContain(
      "required status check",
    );
  });

  it("does not claim a check failed when no CI is configured (empty checks_state)", () => {
    // Regression: `checks_state !== "success"` also matched "" and falsely
    // reported a status-check block on a code-owners/conversation gate.
    const msg = blockedReason(makePR({ checks_state: "" }), t).toLowerCase();
    expect(msg).not.toContain("required status check");
    expect(msg).toContain("repository rules");
  });

  it("falls back to a generic protection note for other rules", () => {
    const msg = blockedReason(
      makePR({ checks_state: "success", required_reviews: 1, review_count: 1 }),
      t,
    );
    expect(msg.toLowerCase()).toContain("repository rules");
  });
});

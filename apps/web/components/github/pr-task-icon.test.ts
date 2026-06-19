import { describe, it, expect } from "vitest";
import {
  aggregatePRStatusColor,
  areAllOpenPRsReadyToMerge,
  getPRStatusColor,
  getPRTooltip,
  isPRAwaitingReview,
  isPRReadyToMerge,
  isPRWaitingOnBranchProtection,
  pickDefaultPR,
  prStatusRank,
} from "./pr-task-icon";
import type { TaskPR } from "@/lib/types/github";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    task_id: "task",
    owner: "o",
    repo: "r",
    pr_number: 1,
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

const SKY_400 = "text-sky-400";
const RED_500 = "text-red-500";
const YELLOW_500 = "text-yellow-500";
const MUTED_FOREGROUND = "text-muted-foreground";

describe("isPRReadyToMerge", () => {
  it("is true when open + approved + success + clean", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          state: "open",
          review_state: "approved",
          checks_state: "success",
          mergeable_state: "clean",
        }),
      ),
    ).toBe(true);
  });

  it("is true when CI succeeds and no reviewers are required (clean + no pending reviews)", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          state: "open",
          review_state: "",
          checks_state: "success",
          mergeable_state: "clean",
          pending_review_count: 0,
        }),
      ),
    ).toBe(true);
  });

  it("is false when reviewers are requested even if CI passed and mergeable is clean", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          state: "open",
          review_state: "pending",
          checks_state: "success",
          mergeable_state: "clean",
          pending_review_count: 2,
        }),
      ),
    ).toBe(false);
  });

  it("is false when no review state but pending reviewers still requested", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          state: "open",
          review_state: "",
          checks_state: "success",
          mergeable_state: "clean",
          pending_review_count: 1,
        }),
      ),
    ).toBe(false);
  });

  it("is false when mergeable_state is blocked", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          state: "open",
          review_state: "approved",
          checks_state: "success",
          mergeable_state: "blocked",
        }),
      ),
    ).toBe(false);
  });

  it("is false when state is merged", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          state: "merged",
          review_state: "approved",
          checks_state: "success",
          mergeable_state: "clean",
        }),
      ),
    ).toBe(false);
  });

  it.each(["behind", "dirty", "has_hooks", "unstable", "draft", "unknown", ""] as const)(
    "is false when mergeable_state is %s",
    (mergeable_state) => {
      expect(
        isPRReadyToMerge(
          makePR({
            state: "open",
            review_state: "approved",
            checks_state: "success",
            mergeable_state,
          }),
        ),
      ).toBe(false);
    },
  );
});

describe("isPRReadyToMerge — required_reviews gate", () => {
  it("is false when required_reviews is unmet even if mergeable_state is clean", () => {
    // GitHub's stored mergeable_state can lag branch-protection state (e.g.
    // after a dismissed approval); the required_reviews gate guarantees the
    // button matches GitHub's merge box.
    expect(
      isPRReadyToMerge(
        makePR({
          state: "open",
          review_state: "approved",
          checks_state: "success",
          mergeable_state: "clean",
          required_reviews: 2,
          review_count: 1,
        }),
      ),
    ).toBe(false);
  });

  it("is true when required_reviews equals review_count and everything else is clean", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          state: "open",
          review_state: "approved",
          checks_state: "success",
          mergeable_state: "clean",
          required_reviews: 2,
          review_count: 2,
        }),
      ),
    ).toBe(true);
  });

  it("is true when required_reviews is zero (protected branch with no approval requirement)", () => {
    expect(
      isPRReadyToMerge(
        makePR({
          state: "open",
          review_state: "",
          checks_state: "success",
          mergeable_state: "clean",
          required_reviews: 0,
          review_count: 0,
          pending_review_count: 0,
        }),
      ),
    ).toBe(true);
  });
});

describe("getPRStatusColor", () => {
  it("returns ready-to-merge color when all conditions are met", () => {
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
    expect(getPRStatusColor(pr)).toBe("text-emerald-400");
  });

  it("returns muted for approved+success but mergeable_state blocked (branch protection)", () => {
    // Branch protection is a normal repository-rule wait after CI passes, not
    // a warning state.
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "blocked",
    });
    expect(getPRStatusColor(pr)).toBe(MUTED_FOREGROUND);
    expect(isPRWaitingOnBranchProtection(pr)).toBe(true);
  });

  it("returns sky-400 for approved PR that still has pending reviewers (1 of N required)", () => {
    // GitHub's review_state="approved" only means at least one reviewer approved;
    // when branch protection requires more reviews, mergeable_state="blocked" and
    // pending_review_count > 0. The icon must not imply the PR is fully approved.
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "blocked",
      pending_review_count: 1,
    });
    expect(getPRStatusColor(pr)).toBe(SKY_400);
  });

  it("returns sky-400 when required_reviews is unmet but mergeable_state is clean (bug repro)", () => {
    // The stale-snapshot bug: GitHub still reports mergeable_state="clean" while
    // branch protection has recorded one fewer approval than required. The icon
    // must downgrade to "awaiting review" instead of the emerald "ready to merge".
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
      required_reviews: 2,
      review_count: 1,
    });
    expect(getPRStatusColor(pr)).toBe(SKY_400);
  });

  it("returns plain green when mergeable_state is empty (backfilled row)", () => {
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "",
    });
    expect(getPRStatusColor(pr)).toBe("text-green-500");
  });

  it("returns sky-400 when CI passed but review is pending", () => {
    const pr = makePR({
      state: "open",
      review_state: "pending",
      checks_state: "success",
      mergeable_state: "clean",
      pending_review_count: 2,
    });
    expect(getPRStatusColor(pr)).toBe(SKY_400);
  });

  it("returns sky-400 when CI passed and reviewers are requested but no review state set", () => {
    const pr = makePR({
      state: "open",
      review_state: "",
      checks_state: "success",
      mergeable_state: "blocked",
      pending_review_count: 1,
    });
    expect(getPRStatusColor(pr)).toBe(SKY_400);
  });

  it("returns emerald when CI passed and no reviewers are required", () => {
    const pr = makePR({
      state: "open",
      review_state: "",
      checks_state: "success",
      mergeable_state: "clean",
      pending_review_count: 0,
    });
    expect(getPRStatusColor(pr)).toBe("text-emerald-400");
  });

  it("returns red for changes_requested regardless of mergeable_state", () => {
    const pr = makePR({
      state: "open",
      review_state: "changes_requested",
      checks_state: "success",
      mergeable_state: "clean",
    });
    expect(getPRStatusColor(pr)).toBe(RED_500);
  });

  it("returns yellow for pending CI", () => {
    const pr = makePR({ state: "open", checks_state: "pending" });
    expect(getPRStatusColor(pr)).toBe(YELLOW_500);
  });

  it("returns purple for merged", () => {
    expect(getPRStatusColor(makePR({ state: "merged" }))).toBe("text-purple-500");
  });
});

describe("getPRStatusColor — mergeability", () => {
  it("returns red for a dirty (merge-conflict) PR even when approved + checks pass", () => {
    // Regression: before the mergeability branch, an approved+success but
    // conflicted PR fell through to plain green — the icon read "good" while
    // the PR could not be merged.
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "dirty",
    });
    expect(getPRStatusColor(pr)).toBe(RED_500);
  });

  it("returns yellow for a behind-base PR that is otherwise approved + green", () => {
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "behind",
    });
    expect(getPRStatusColor(pr)).toBe(YELLOW_500);
  });

  it("ignores dirty/behind mergeable_state on terminal (merged) PRs", () => {
    expect(getPRStatusColor(makePR({ state: "merged", mergeable_state: "dirty" }))).toBe(
      "text-purple-500",
    );
  });
});

describe("isPRAwaitingReview", () => {
  it("is true when CI succeeded and review is pending", () => {
    expect(
      isPRAwaitingReview(
        makePR({
          state: "open",
          review_state: "pending",
          checks_state: "success",
          pending_review_count: 1,
        }),
      ),
    ).toBe(true);
  });

  it("is false when CI is still running", () => {
    expect(
      isPRAwaitingReview(
        makePR({ state: "open", checks_state: "pending", pending_review_count: 1 }),
      ),
    ).toBe(false);
  });

  it("is false when no review is required", () => {
    expect(
      isPRAwaitingReview(
        makePR({
          state: "open",
          review_state: "",
          checks_state: "success",
          pending_review_count: 0,
        }),
      ),
    ).toBe(false);
  });

  it("is true for an approved PR with extra reviewers still pending", () => {
    // One reviewer approved but branch protection requires more — still awaiting.
    expect(
      isPRAwaitingReview(
        makePR({
          state: "open",
          review_state: "approved",
          checks_state: "success",
          pending_review_count: 1,
        }),
      ),
    ).toBe(true);
  });

  it("is false for an approved PR with no pending reviewers", () => {
    expect(
      isPRAwaitingReview(
        makePR({
          state: "open",
          review_state: "approved",
          checks_state: "success",
          pending_review_count: 0,
        }),
      ),
    ).toBe(false);
  });

  it("is true when required_reviews is unmet even with no pending reviewers", () => {
    // 1 of 2 approvals; the second reviewer is no longer requested but branch
    // protection still demands two approvals — surface as awaiting review.
    expect(
      isPRAwaitingReview(
        makePR({
          state: "open",
          review_state: "approved",
          checks_state: "success",
          pending_review_count: 0,
          required_reviews: 2,
          review_count: 1,
        }),
      ),
    ).toBe(true);
  });
});

describe("aggregatePRStatusColor", () => {
  it("returns muted for empty list", () => {
    expect(aggregatePRStatusColor([])).toBe(MUTED_FOREGROUND);
  });

  it("surfaces the worst-of state — one red dominates a green sibling", () => {
    const green = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
    const red = makePR({
      state: "open",
      review_state: "changes_requested",
      checks_state: "success",
    });
    expect(aggregatePRStatusColor([green, red])).toBe(RED_500);
  });

  it("returns emerald only when all PRs are ready to merge", () => {
    const ready = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
    expect(aggregatePRStatusColor([ready, ready])).toBe("text-emerald-400");
  });

  it("yellow CI pending beats merged purple", () => {
    const pending = makePR({ state: "open", checks_state: "pending" });
    const merged = makePR({ state: "merged" });
    expect(aggregatePRStatusColor([merged, pending])).toBe(YELLOW_500);
  });

  it("ignores a merged PR when a fresh open PR has no status yet", () => {
    // Repro: first PR merged, then a new branch + PR opened on the same task.
    // The open PR has no checks/reviews yet so its color rank ties merged at
    // 0, but the icon must reflect the live PR, not the closed one.
    const merged = makePR({ state: "merged" });
    const fresh = makePR({ state: "open", review_state: "", checks_state: "" });
    expect(aggregatePRStatusColor([merged, fresh])).toBe(MUTED_FOREGROUND);
  });

  it("ignores a closed PR when a fresh open PR is present", () => {
    // Closed PRs rank red (5, the highest), so without filtering they would
    // dominate every open sibling. Same reasoning as the merged case — a
    // terminal PR shouldn't drive the live status indicator.
    const closed = makePR({ state: "closed" });
    const fresh = makePR({ state: "open", review_state: "", checks_state: "" });
    expect(aggregatePRStatusColor([closed, fresh])).toBe(MUTED_FOREGROUND);
  });

  it("falls back to terminal state when every PR is merged/closed", () => {
    // All-terminal tasks bypass the open-PR filter and rank across every PR,
    // so the result depends on which terminal state is present. Merged ranks
    // 0 (purple), closed ranks 5 (red) — both paths need a guard.
    const merged = makePR({ state: "merged" });
    expect(aggregatePRStatusColor([merged, merged])).toBe("text-purple-500");
    const closed = makePR({ state: "closed" });
    expect(aggregatePRStatusColor([closed, closed])).toBe(RED_500);
  });
});

describe("areAllOpenPRsReadyToMerge", () => {
  const ready = () =>
    makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });

  it("is false when the list is empty", () => {
    expect(areAllOpenPRsReadyToMerge([])).toBe(false);
  });

  it("is false when no PR is open", () => {
    // A fully-merged task isn't "ready to merge" — there's nothing left to do.
    expect(areAllOpenPRsReadyToMerge([makePR({ state: "merged" })])).toBe(false);
  });

  it("is true when the only open PR is ready, even with a merged sibling", () => {
    // The fix: a merged sibling used to drag the result to false because
    // prs.every(isPRReadyToMerge) saw the merged PR as not-ready.
    expect(areAllOpenPRsReadyToMerge([makePR({ state: "merged" }), ready()])).toBe(true);
  });

  it("is false when any open PR is not ready", () => {
    const pending = makePR({ state: "open", checks_state: "pending" });
    expect(areAllOpenPRsReadyToMerge([ready(), pending])).toBe(false);
  });

  it("is true when every open PR is ready", () => {
    expect(areAllOpenPRsReadyToMerge([ready(), ready()])).toBe(true);
  });
});

describe("prStatusRank", () => {
  it("returns -1 for terminal PRs so they're never the default focus", () => {
    expect(prStatusRank(makePR({ state: "merged" }))).toBe(-1);
    expect(prStatusRank(makePR({ state: "closed" }))).toBe(-1);
  });

  it("ranks a failing open PR above a passing open PR", () => {
    const failing = makePR({ state: "open", checks_state: "failure" });
    const passing = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
    expect(prStatusRank(failing)).toBeGreaterThan(prStatusRank(passing));
  });
});

describe("pickDefaultPR", () => {
  it("returns null for an empty list", () => {
    expect(pickDefaultPR([])).toBeNull();
  });

  it("picks the worst-status open PR (failing over passing)", () => {
    const passing = makePR({
      id: "pass",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
    const failing = makePR({ id: "fail", state: "open", checks_state: "failure" });
    expect(pickDefaultPR([passing, failing])?.id).toBe("fail");
  });

  it("prefers an open PR over a terminal one even when listed first", () => {
    const merged = makePR({ id: "merged", state: "merged" });
    const open = makePR({ id: "open", state: "open", checks_state: "pending" });
    expect(pickDefaultPR([merged, open])?.id).toBe("open");
  });

  it("breaks ties on the first PR (creation order)", () => {
    const first = makePR({ id: "first", state: "open", checks_state: "failure" });
    const second = makePR({ id: "second", state: "open", checks_state: "failure" });
    expect(pickDefaultPR([first, second])?.id).toBe("first");
  });

  it("falls back to the first PR when every PR is terminal", () => {
    const merged = makePR({ id: "merged", state: "merged" });
    const closed = makePR({ id: "closed", state: "closed" });
    expect(pickDefaultPR([merged, closed])?.id).toBe("merged");
  });
});

describe("getPRTooltip", () => {
  it("includes 'Ready to merge' when ready", () => {
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
    });
    expect(getPRTooltip(pr)).toContain("Ready to merge");
  });

  it("includes 'Mergeable: blocked' when blocked", () => {
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "blocked",
    });
    expect(getPRTooltip(pr)).toContain("Mergeable: blocked");
    expect(getPRTooltip(pr)).not.toContain("Ready to merge");
  });

  it("omits mergeable when state is empty or unknown", () => {
    const empty = makePR({ state: "open", mergeable_state: "" });
    const unknown = makePR({ state: "open", mergeable_state: "unknown" });
    expect(getPRTooltip(empty)).not.toContain("Mergeable:");
    expect(getPRTooltip(unknown)).not.toContain("Mergeable:");
  });
});

import { describe, it, expect } from "vitest";
import {
  getPRStatusColor,
  getPRTooltip,
  isPRAwaitingReview,
  isPRReadyToMerge,
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

  it("returns plain green for approved+success but mergeable_state blocked", () => {
    const pr = makePR({
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "blocked",
    });
    expect(getPRStatusColor(pr)).toBe("text-green-500");
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
    expect(getPRStatusColor(pr)).toBe("text-sky-400");
  });

  it("returns sky-400 when CI passed and reviewers are requested but no review state set", () => {
    const pr = makePR({
      state: "open",
      review_state: "",
      checks_state: "success",
      mergeable_state: "blocked",
      pending_review_count: 1,
    });
    expect(getPRStatusColor(pr)).toBe("text-sky-400");
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
    expect(getPRStatusColor(pr)).toBe("text-red-500");
  });

  it("returns yellow for pending CI", () => {
    const pr = makePR({ state: "open", checks_state: "pending" });
    expect(getPRStatusColor(pr)).toBe("text-yellow-500");
  });

  it("returns purple for merged", () => {
    expect(getPRStatusColor(makePR({ state: "merged" }))).toBe("text-purple-500");
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

  it("is false for an approved PR with extra reviewers still pending", () => {
    expect(
      isPRAwaitingReview(
        makePR({
          state: "open",
          review_state: "approved",
          checks_state: "success",
          pending_review_count: 1,
        }),
      ),
    ).toBe(false);
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

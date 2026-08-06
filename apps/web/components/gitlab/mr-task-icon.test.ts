import { describe, expect, it } from "vitest";
import {
  aggregateMRStatusColor,
  getMRStatusColor,
  getMRTooltip,
  isMRReadyToMerge,
} from "./mr-task-icon";
import type { TaskMR } from "@/lib/types/gitlab";

const MUTED = "text-muted-foreground";

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "id",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "group/project",
    mr_iid: 1,
    mr_url: "",
    mr_title: "Test MR",
    head_branch: "feature",
    base_branch: "main",
    author_username: "alice",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    reviewer_count: 0,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

// AC35: one case per row of the priority table, plus order-proving cases.
describe("getMRStatusColor", () => {
  it.each([
    ["merged", makeMR({ state: "merged" }), "text-purple-500"],
    ["closed", makeMR({ state: "closed" }), MUTED],
    ["locked", makeMR({ state: "locked" }), MUTED],
    ["pipeline failure", makeMR({ pipeline_state: "failure" }), "text-red-500"],
    ["draft", makeMR({ draft: true }), MUTED],
    [
      "approved and pipeline success",
      makeMR({ approval_state: "approved", pipeline_state: "success" }),
      "text-emerald-400",
    ],
    ["approval pending", makeMR({ approval_state: "pending" }), "text-sky-400"],
    ["pipeline pending", makeMR({ pipeline_state: "pending" }), "text-yellow-500"],
    ["otherwise", makeMR(), MUTED],
  ])("%s -> %s", (_name, mr, expected) => {
    expect(getMRStatusColor(mr)).toBe(expected);
  });

  it("order: merged beats a failing pipeline (row 1 before row 4)", () => {
    const mr = makeMR({ state: "merged", pipeline_state: "failure" });
    expect(getMRStatusColor(mr)).toBe("text-purple-500");
  });

  it("order: closed beats approved (row 2 before row 6)", () => {
    const mr = makeMR({ state: "closed", approval_state: "approved", pipeline_state: "success" });
    expect(getMRStatusColor(mr)).toBe(MUTED);
  });

  it("order: draft beats approved+success (row 5 before row 6) — draft never reads as done", () => {
    const mr = makeMR({ draft: true, approval_state: "approved", pipeline_state: "success" });
    expect(getMRStatusColor(mr)).toBe(MUTED);
  });

  it("order: a failing pipeline beats approved (row 4 before row 6) — approved with red CI is not emerald", () => {
    const mr = makeMR({ approval_state: "approved", pipeline_state: "failure" });
    expect(getMRStatusColor(mr)).toBe("text-red-500");
  });

  it("order: a running pipeline beats approved (row 8 is only reached if row 6 didn't match) — approved with pending CI is not emerald", () => {
    const mr = makeMR({ approval_state: "approved", pipeline_state: "pending" });
    expect(getMRStatusColor(mr)).toBe("text-yellow-500");
  });
});

// AC37: colour is never the only carrier of MR state — the tooltip text
// states it explicitly for any non-open MR.
describe("getMRTooltip", () => {
  it("states the MR state for a non-open MR", () => {
    expect(getMRTooltip(makeMR({ state: "merged" }))).toContain("State: merged");
    expect(getMRTooltip(makeMR({ state: "closed" }))).toContain("State: closed");
  });

  it("omits the state line for an open MR", () => {
    expect(getMRTooltip(makeMR({ state: "open" }))).not.toContain("State:");
  });

  it("flags a ready-to-merge MR", () => {
    expect(
      getMRTooltip(
        makeMR({
          approval_state: "approved",
          pipeline_state: "success",
          merge_status: "can_be_merged",
        }),
      ),
    ).toContain("Ready to merge");
  });

  it("flags a draft MR without claiming it's ready to merge", () => {
    const tooltip = getMRTooltip(makeMR({ draft: true }));
    expect(tooltip).toContain("Draft");
    expect(tooltip).not.toContain("Ready to merge");
  });
});

describe("isMRReadyToMerge", () => {
  it("is true only for open, non-draft, approved MRs with a passing pipeline and a mergeable status", () => {
    expect(
      isMRReadyToMerge(
        makeMR({
          approval_state: "approved",
          pipeline_state: "success",
          merge_status: "can_be_merged",
        }),
      ),
    ).toBe(true);
    expect(
      isMRReadyToMerge(
        makeMR({ draft: true, approval_state: "approved", pipeline_state: "success" }),
      ),
    ).toBe(false);
    expect(isMRReadyToMerge(makeMR({ state: "merged", approval_state: "approved" }))).toBe(false);
  });

  it("is false when there are unresolved discussions, even if every other gate passes", () => {
    expect(
      isMRReadyToMerge(
        makeMR({
          approval_state: "approved",
          pipeline_state: "success",
          merge_status: "can_be_merged",
          unresolved_discussions: 1,
        }),
      ),
    ).toBe(false);
  });

  it("is false when detailed_merge_status is present but not mergeable, regardless of merge_status", () => {
    expect(
      isMRReadyToMerge(
        makeMR({
          approval_state: "approved",
          pipeline_state: "success",
          merge_status: "can_be_merged",
          detailed_merge_status: "not_approved",
        }),
      ),
    ).toBe(false);
  });

  it("falls back to merge_status when detailed_merge_status is absent (pre-15.6 host)", () => {
    expect(
      isMRReadyToMerge(
        makeMR({
          approval_state: "approved",
          pipeline_state: "success",
          merge_status: "can_be_merged",
        }),
      ),
    ).toBe(true);
    expect(
      isMRReadyToMerge(
        makeMR({
          approval_state: "approved",
          pipeline_state: "success",
          merge_status: "cannot_be_merged",
        }),
      ),
    ).toBe(false);
  });
});

// AC28: worst open MR wins; terminal MRs dropped when any is open; all-terminal falls back to the first.
describe("aggregateMRStatusColor", () => {
  it("returns muted for an empty list", () => {
    expect(aggregateMRStatusColor([])).toBe(MUTED);
  });

  it("picks the worst (highest-rank) colour among open MRs", () => {
    const mrs = [
      makeMR({ id: "a", approval_state: "approved", pipeline_state: "success" }),
      makeMR({ id: "b", pipeline_state: "failure" }),
    ];
    expect(aggregateMRStatusColor(mrs)).toBe("text-red-500");
  });

  it("drops terminal MRs when at least one MR is open", () => {
    const mrs = [
      makeMR({ id: "a", state: "merged" }),
      makeMR({ id: "b", state: "open", pipeline_state: "pending" }),
    ];
    expect(aggregateMRStatusColor(mrs)).toBe("text-yellow-500");
  });

  it("falls back to the first MR's colour when every MR is terminal", () => {
    const mrs = [makeMR({ id: "a", state: "merged" }), makeMR({ id: "b", state: "closed" })];
    expect(aggregateMRStatusColor(mrs)).toBe("text-purple-500");
  });
});

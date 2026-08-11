import { describe, expect, it } from "vitest";
import {
  aggregateMRChipStatus,
  aggregateMRStatusColor,
  compareChipMR,
  getMRStatusColor,
  getMRTooltip,
  isMRReadyToMerge,
  mrChipStatus,
  MR_CHIP_STATUS_RANK,
  STATUS_RANK,
  selectChipMR,
  type MRChipStatus,
} from "./mr-task-icon";
import type { TaskMR } from "@/lib/types/gitlab";

const MUTED = "text-muted-foreground";
const PURPLE = "text-purple-500";
const RED = "text-red-500";
const YELLOW = "text-yellow-500";

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
    ["merged", makeMR({ state: "merged" }), PURPLE],
    ["closed", makeMR({ state: "closed" }), MUTED],
    ["locked", makeMR({ state: "locked" }), MUTED],
    ["pipeline failure", makeMR({ pipeline_state: "failure" }), RED],
    ["draft", makeMR({ draft: true }), MUTED],
    [
      "approved and pipeline success",
      makeMR({ approval_state: "approved", pipeline_state: "success" }),
      "text-emerald-400",
    ],
    ["approval pending", makeMR({ approval_state: "pending" }), "text-sky-400"],
    ["pipeline pending", makeMR({ pipeline_state: "pending" }), YELLOW],
    ["otherwise", makeMR(), MUTED],
  ])("%s -> %s", (_name, mr, expected) => {
    expect(getMRStatusColor(mr)).toBe(expected);
  });

  it("order: merged beats a failing pipeline (row 1 before row 4)", () => {
    const mr = makeMR({ state: "merged", pipeline_state: "failure" });
    expect(getMRStatusColor(mr)).toBe(PURPLE);
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
    expect(getMRStatusColor(mr)).toBe(RED);
  });

  it("order: a running pipeline beats approved (row 8 is only reached if row 6 didn't match) — approved with pending CI is not emerald", () => {
    const mr = makeMR({ approval_state: "approved", pipeline_state: "pending" });
    expect(getMRStatusColor(mr)).toBe(YELLOW);
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
  it("is true only for open, non-draft MRs with a passing pipeline and a mergeable status", () => {
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

  it("does not gate on approval_state, matching the backend's mrAutomationReadyToMerge (which trusts GitLab's own merge-readiness verdict)", () => {
    expect(
      isMRReadyToMerge(
        makeMR({
          approval_state: "",
          pipeline_state: "success",
          detailed_merge_status: "mergeable",
          unresolved_discussions: 0,
        }),
      ),
    ).toBe(true);
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
    expect(aggregateMRStatusColor(mrs)).toBe(RED);
  });

  it("drops terminal MRs when at least one MR is open", () => {
    const mrs = [
      makeMR({ id: "a", state: "merged" }),
      makeMR({ id: "b", state: "open", pipeline_state: "pending" }),
    ];
    expect(aggregateMRStatusColor(mrs)).toBe(YELLOW);
  });

  it("falls back to the first MR's colour when every MR is terminal", () => {
    const mrs = [makeMR({ id: "a", state: "merged" }), makeMR({ id: "b", state: "closed" })];
    expect(aggregateMRStatusColor(mrs)).toBe(PURPLE);
  });

  // Spec Constraints: "SHALL include at least one all-terminal pair in both
  // input orders" — pins the preserved first-in-input-order tie behaviour
  // that selectChipMR (below) deliberately does NOT share (spec round-4
  // finding F30).
  it("all-terminal pair: array order decides the tie, in both input orders", () => {
    const mergedFirst = [
      makeMR({ id: "a", state: "merged" }),
      makeMR({ id: "b", state: "closed" }),
    ];
    const closedFirst = [
      makeMR({ id: "b", state: "closed" }),
      makeMR({ id: "a", state: "merged" }),
    ];
    expect(aggregateMRStatusColor(mergedFirst)).toBe(PURPLE);
    expect(aggregateMRStatusColor(closedFirst)).toBe(MUTED);
  });
});

// spec: State machine — mrChipStatus is the one priority table
// getMRStatusColor now reads from.
describe("mrChipStatus", () => {
  it.each([
    ["merged", makeMR({ state: "merged" }), "merged"],
    ["closed", makeMR({ state: "closed" }), "closed"],
    ["locked", makeMR({ state: "locked" }), "closed"],
    ["pipeline failure", makeMR({ pipeline_state: "failure" }), "failed"],
    ["draft", makeMR({ draft: true }), "draft"],
    [
      "approved and pipeline success",
      makeMR({ approval_state: "approved", pipeline_state: "success" }),
      "ready",
    ],
    ["approval pending", makeMR({ approval_state: "pending" }), "awaiting_approval"],
    ["pipeline pending", makeMR({ pipeline_state: "pending" }), "running"],
    ["otherwise", makeMR(), "neutral"],
  ] satisfies Array<[string, TaskMR, MRChipStatus]>)("%s -> %s", (_name, mr, expected) => {
    expect(mrChipStatus(mr)).toBe(expected);
  });

  it("row 6 carries no pipeline condition: pending approval + pending pipeline is awaiting_approval, not running", () => {
    expect(mrChipStatus(makeMR({ approval_state: "pending", pipeline_state: "pending" }))).toBe(
      "awaiting_approval",
    );
  });

  it("getMRStatusColor is a byte-identical lookup from mrChipStatus for every status", () => {
    const statuses: MRChipStatus[] = [
      "merged",
      "closed",
      "failed",
      "draft",
      "ready",
      "awaiting_approval",
      "running",
      "neutral",
    ];
    const fixtures: Record<MRChipStatus, TaskMR> = {
      merged: makeMR({ state: "merged" }),
      closed: makeMR({ state: "closed" }),
      failed: makeMR({ pipeline_state: "failure" }),
      draft: makeMR({ draft: true }),
      ready: makeMR({ approval_state: "approved", pipeline_state: "success" }),
      awaiting_approval: makeMR({ approval_state: "pending" }),
      running: makeMR({ pipeline_state: "pending" }),
      neutral: makeMR(),
    };
    for (const status of statuses) {
      expect(mrChipStatus(fixtures[status])).toBe(status);
    }
  });
});

// spec: Selection and ordering.
describe("selectChipMR", () => {
  it("returns null when no MR is open", () => {
    expect(selectChipMR([makeMR({ state: "merged" }), makeMR({ state: "closed" })])).toBeNull();
  });

  it("'open' is a positive equality test: locked and unknown states are not selectable", () => {
    const mrs = [makeMR({ id: "a", state: "locked" }), makeMR({ id: "b", state: "weird-state" })];
    expect(selectChipMR(mrs)).toBeNull();
  });

  it("keeps the maximum MR_CHIP_STATUS_RANK among open MRs", () => {
    const running = makeMR({ id: "a", pipeline_state: "pending" });
    const failed = makeMR({ id: "b", pipeline_state: "failure" });
    expect(selectChipMR([running, failed])?.id).toBe("b");
  });

  it("tiebreaks by mr_iid ascending", () => {
    const mrs = [makeMR({ id: "a", mr_iid: 12 }), makeMR({ id: "b", mr_iid: 7 })];
    expect(selectChipMR(mrs)?.mr_iid).toBe(7);
  });

  it("tiebreaks by project_path ascending when mr_iid ties", () => {
    const mrs = [
      makeMR({ id: "a", mr_iid: 7, project_path: "group/b" }),
      makeMR({ id: "b", mr_iid: 7, project_path: "group/a" }),
    ];
    expect(selectChipMR(mrs)?.project_path).toBe("group/a");
  });

  it("tiebreaks by id ascending when mr_iid and project_path both tie", () => {
    const mrs = [
      makeMR({ id: "z", mr_iid: 7, project_path: "group/a" }),
      makeMR({ id: "a", mr_iid: 7, project_path: "group/a" }),
    ];
    expect(selectChipMR(mrs)?.id).toBe("a");
  });

  it("is order-independent: the same winner is picked regardless of input array order", () => {
    const draft = makeMR({ id: "draft", mr_iid: 9, draft: true });
    const neutral = makeMR({ id: "neutral", mr_iid: 3 });
    expect(selectChipMR([draft, neutral])?.id).toBe("neutral");
    expect(selectChipMR([neutral, draft])?.id).toBe("neutral");
  });

  // spec round-4 finding F30: selectChipMR must not mutate its input, since
  // useTaskMRs returns the store array by reference and that same array
  // backs aggregateMRStatusColor's array-order-dependent tie.
  it("does not mutate the input array's order", () => {
    const mrs = [makeMR({ id: "b", mr_iid: 2 }), makeMR({ id: "a", mr_iid: 1 })];
    const before = mrs.map((mr) => mr.id);
    selectChipMR(mrs);
    expect(mrs.map((mr) => mr.id)).toEqual(before);
  });
});

describe("aggregateMRChipStatus", () => {
  it("equals mrChipStatus(selectChipMR(mrs)) for a mixed list", () => {
    const mrs = [
      makeMR({ id: "a", pipeline_state: "pending" }),
      makeMR({ id: "b", pipeline_state: "failure" }),
    ];
    expect(aggregateMRChipStatus(mrs)).toBe(mrChipStatus(selectChipMR(mrs) as TaskMR));
  });

  it("is neutral when selectChipMR returns null", () => {
    expect(aggregateMRChipStatus([makeMR({ state: "merged" })])).toBe("neutral");
  });

  it("is order-independent over shuffled input", () => {
    const mrs = [
      makeMR({ id: "a", mr_iid: 3 }),
      makeMR({ id: "b", mr_iid: 9, draft: true }),
      makeMR({ id: "c", mr_iid: 1, pipeline_state: "failure" }),
    ];
    const shuffled = [mrs[2], mrs[0], mrs[1]];
    expect(aggregateMRChipStatus(mrs)).toBe(aggregateMRChipStatus(shuffled));
    expect(aggregateMRChipStatus(mrs)).toBe("failed");
  });
});

// spec: State machine, "Rank, and why aggregation has no tiebreak of its
// own" — MR_CHIP_STATUS_RANK and STATUS_RANK are two separate tables keyed
// on different things, required only to be order-equivalent under the
// status-to-colour map (spec round-4 finding F34).
describe("MR_CHIP_STATUS_RANK / STATUS_RANK order-equivalence", () => {
  const colorFor: Record<MRChipStatus, string> = {
    merged: PURPLE,
    closed: MUTED,
    failed: RED,
    draft: MUTED,
    ready: "text-emerald-400",
    awaiting_approval: "text-sky-400",
    running: YELLOW,
    neutral: MUTED,
  };
  const statuses = Object.keys(MR_CHIP_STATUS_RANK) as MRChipStatus[];

  it("orders any two statuses the same way as STATUS_RANK does their colours", () => {
    for (const a of statuses) {
      for (const b of statuses) {
        const chipOrder = MR_CHIP_STATUS_RANK[a] < MR_CHIP_STATUS_RANK[b];
        const colorOrder = STATUS_RANK[colorFor[a]] < STATUS_RANK[colorFor[b]];
        expect(chipOrder).toBe(colorOrder);
      }
    }
  });
});

describe("compareChipMR", () => {
  it("is a total order: exactly one of a<b, a===b, a>b holds for any pair", () => {
    const a = makeMR({ id: "a", mr_iid: 1, project_path: "group/a" });
    const b = makeMR({ id: "b", mr_iid: 1, project_path: "group/a" });
    expect(compareChipMR(a, b)).toBeLessThan(0);
    expect(compareChipMR(b, a)).toBeGreaterThan(0);
    expect(compareChipMR(a, a)).toBe(0);
  });
});

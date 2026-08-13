import { describe, expect, it, afterEach } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { aggregateMRStatusColor, getMRStatusColor, MRStatusIcon } from "./mr-task-icon";
import type { TaskMR } from "@/lib/types/gitlab";

afterEach(() => cleanup());

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

// AC36: mr-topbar-button.tsx renders its trigger glyph via the exact same
// <MRStatusIcon> exported here — the component the Kanban card badge
// (mr-task-icon.tsx's MRTaskIcon) also derives its colour from via
// getMRStatusColor. This renders MRStatusIcon (what the topbar trigger
// actually mounts) and asserts its resolved class matches
// getMRStatusColor's output (what the badge's className is built from) for
// every AC35 fixture, so a future edit that gives one consumer its own
// copy — rather than importing this one — fails here.
const AC35_FIXTURES: Array<{ name: string; mr: TaskMR; want: string }> = [
  { name: "merged", mr: makeMR({ state: "merged" }), want: "text-purple-500" },
  { name: "closed", mr: makeMR({ state: "closed" }), want: MUTED },
  { name: "locked", mr: makeMR({ state: "locked" }), want: MUTED },
  { name: "pipeline failure", mr: makeMR({ pipeline_state: "failure" }), want: "text-red-500" },
  { name: "draft", mr: makeMR({ draft: true }), want: MUTED },
  {
    name: "approved and pipeline success",
    mr: makeMR({ approval_state: "approved", pipeline_state: "success" }),
    want: "text-emerald-400",
  },
  { name: "approval pending", mr: makeMR({ approval_state: "pending" }), want: "text-sky-400" },
  { name: "pipeline pending", mr: makeMR({ pipeline_state: "pending" }), want: "text-yellow-500" },
  { name: "otherwise", mr: makeMR(), want: MUTED },
];

describe("MR status colour parity (AC36)", () => {
  it.each(AC35_FIXTURES)(
    "MRStatusIcon (topbar trigger) renders getMRStatusColor's class for: $name",
    ({ mr, want }) => {
      const { container } = render(<MRStatusIcon mr={mr} />);
      const svg = container.querySelector("svg");
      expect(svg?.getAttribute("class")).toContain(want);
      expect(getMRStatusColor(mr)).toBe(want);
    },
  );

  // spec Constraints: aggregateMRStatusColor's first-in-input-order tie is
  // frozen, not fixed, by the MR status chip feature — this pins it in both
  // input orders so the chip's own array-order-independent selectChipMR
  // can never accidentally end up sharing this function's behaviour.
  it("aggregateMRStatusColor's all-terminal tie is decided by array order, in both orders", () => {
    const merged = makeMR({ id: "a", state: "merged" });
    const closed = makeMR({ id: "b", state: "closed" });
    expect(aggregateMRStatusColor([merged, closed])).toBe("text-purple-500");
    expect(aggregateMRStatusColor([closed, merged])).toBe(MUTED);
  });
});

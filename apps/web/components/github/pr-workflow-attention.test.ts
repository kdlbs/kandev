import { describe, expect, it } from "vitest";
import type { TaskPR, WorkflowAttention } from "@/lib/types/github";
import {
  getActiveWorkflowAttention,
  getCurrentWorkflowAttention,
  getWorkflowAttentionForDisplay,
} from "./pr-workflow-attention";

function makeAttention(overrides: Partial<WorkflowAttention> = {}): WorkflowAttention {
  return {
    state: "approval_required",
    head_sha: "head-1",
    observed_at: "2026-09-10T10:00:00Z",
    stale: false,
    runs: [
      {
        run_id: 7,
        run_attempt: 1,
        workflow_id: 9,
        name: "Run tests",
        url: "https://github.com/acme/widget/actions/runs/7",
        reason: "approval_required",
      },
    ],
    ...overrides,
  };
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-id",
    workspace_id: "workspace-1",
    task_id: "task-id",
    owner: "acme",
    repo: "widget",
    pr_number: 143,
    pr_url: "https://github.com/acme/widget/pull/143",
    pr_title: "Workflow attention",
    head_branch: "feat/workflow",
    base_branch: "main",
    author_login: "octocat",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "unstable",
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
    head_sha: "head-1",
    ...overrides,
  };
}

describe("getActiveWorkflowAttention", () => {
  it("returns current approval evidence, including stale same-head evidence", () => {
    const attention = makeAttention({ stale: true });

    expect(getActiveWorkflowAttention(makePR({ workflow_attention: attention }))).toBe(attention);
  });

  it("rejects legacy, unknown, terminal, and changed-head observations", () => {
    expect(getActiveWorkflowAttention(makePR())).toBeNull();
    expect(
      getActiveWorkflowAttention(
        makePR({ workflow_attention: makeAttention({ state: "unknown" }) }),
      ),
    ).toBeNull();
    expect(
      getActiveWorkflowAttention(makePR({ workflow_attention: makeAttention({ state: "none" }) })),
    ).toBeNull();
    expect(
      getActiveWorkflowAttention(
        makePR({ workflow_attention: makeAttention({ head_sha: "old-head" }) }),
      ),
    ).toBeNull();
    expect(
      getActiveWorkflowAttention(makePR({ state: "merged", workflow_attention: makeAttention() })),
    ).toBeNull();
  });

  it("shows unavailable evidence and preserves a stored positive result during a live read failure", () => {
    const unknown = makeAttention({ state: "unknown", runs: [] });
    const stored = makeAttention({ stale: false });
    const carrier = makePR({ workflow_attention: stored });

    expect(getWorkflowAttentionForDisplay(makePR({ workflow_attention: unknown }))).toEqual(
      unknown,
    );
    expect(getWorkflowAttentionForDisplay(carrier, unknown)).toEqual({
      ...stored,
      stale: true,
    });
    expect(
      getWorkflowAttentionForDisplay(carrier, makeAttention({ state: "none", runs: [] })),
    ).toBeNull();
  });

  it("does not render an authoritative empty result and normalizes legacy null runs", () => {
    expect(
      getWorkflowAttentionForDisplay(
        makePR({ workflow_attention: makeAttention({ state: "none", runs: [] }) }),
      ),
    ).toBeNull();
    expect(
      getWorkflowAttentionForDisplay(
        makePR({ workflow_attention: makeAttention({ state: "unknown", runs: null as never }) }),
      ),
    ).toEqual(makeAttention({ state: "unknown", runs: [] }));
  });
});

describe("getCurrentWorkflowAttention", () => {
  it("normalizes legacy null run arrays for summary consumers", () => {
    const current = getCurrentWorkflowAttention(
      makePR({ workflow_attention: makeAttention({ runs: null as never }) }),
    );

    expect(current?.runs).toEqual([]);
  });
});

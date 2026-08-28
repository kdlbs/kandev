import { describe, expect, it } from "vitest";
import { getTaskPRAutomationSummary } from "./pr-task-icon";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "id",
    workspace_id: "workspace-1",
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

function makeOptions(): TaskCIAutomationOptions {
  return {
    task_id: "task",
    workspace_id: "workspace-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    updated_at: "2026-08-01T00:00:00Z",
    pr_states: [],
    pr_options: [
      {
        task_id: "task",
        repository_id: "repo-a",
        pr_number: 1,
        auto_fix_enabled: true,
        auto_merge_enabled: false,
        prompt_on_review_requested: false,
        prompt_on_merged: false,
        prompt_on_closed: false,
        created_at: "",
        updated_at: "",
      },
      {
        task_id: "task",
        repository_id: "repo-b",
        pr_number: 2,
        auto_fix_enabled: false,
        auto_merge_enabled: true,
        prompt_on_review_requested: false,
        prompt_on_merged: false,
        prompt_on_closed: false,
        created_at: "",
        updated_at: "",
      },
      {
        task_id: "task",
        repository_id: "repo-c",
        pr_number: 3,
        auto_fix_enabled: true,
        auto_merge_enabled: true,
        prompt_on_review_requested: false,
        prompt_on_merged: false,
        prompt_on_closed: false,
        created_at: "",
        updated_at: "",
      },
    ],
  };
}

describe("getTaskPRAutomationSummary", () => {
  it("aggregates independent active PR settings and ignores terminal PRs", () => {
    const summary = getTaskPRAutomationSummary(
      [
        makePR({ repository_id: "repo-a", pr_number: 1 }),
        makePR({ repository_id: "repo-b", pr_number: 2 }),
        makePR({ repository_id: "repo-c", pr_number: 3, state: "merged" }),
      ],
      undefined,
      makeOptions(),
    );

    expect(summary).toEqual({
      autoFixEnabled: true,
      autoMergeEnabled: true,
      details: [
        { number: 1, repository: "o/r", autoFixEnabled: true, autoMergeEnabled: false },
        { number: 2, repository: "o/r", autoFixEnabled: false, autoMergeEnabled: true },
      ],
    });
  });

  it("uses bounded row flags before per-PR details hydrate", () => {
    expect(
      getTaskPRAutomationSummary([], {
        number: 7,
        state: "open",
        autoFixEnabled: true,
        autoMergeEnabled: true,
      }),
    ).toEqual({
      autoFixEnabled: true,
      autoMergeEnabled: true,
      details: [],
    });
  });

  it("keeps bounded row flags while full PR records are temporarily unavailable", () => {
    expect(
      getTaskPRAutomationSummary(
        [],
        { number: 7, state: "open", autoFixEnabled: true, autoMergeEnabled: false },
        makeOptions(),
      ),
    ).toEqual({
      autoFixEnabled: true,
      autoMergeEnabled: false,
      details: [],
    });
  });

  it("retains repository identity when linked PR numbers overlap", () => {
    const summary = getTaskPRAutomationSummary(
      [
        makePR({ owner: "acme", repo: "frontend", repository_id: "repo-a", pr_number: 7 }),
        makePR({ owner: "acme", repo: "backend", repository_id: "repo-b", pr_number: 7 }),
      ],
      undefined,
      {
        ...makeOptions(),
        pr_options: [
          { ...makeOptions().pr_options[0], repository_id: "repo-a", pr_number: 7 },
          { ...makeOptions().pr_options[1], repository_id: "repo-b", pr_number: 7 },
        ],
      },
    );

    expect(summary.details.map(({ repository, number }) => ({ repository, number }))).toEqual([
      { repository: "acme/frontend", number: 7 },
      { repository: "acme/backend", number: 7 },
    ]);
  });
});

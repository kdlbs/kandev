import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

export function makeTestPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-id",
    workspace_id: "workspace-1",
    task_id: "task-1",
    owner: "acme",
    repo: "demo",
    pr_number: 42,
    pr_url: "https://github.com/acme/demo/pull/42",
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 1,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 2,
    checks_passing: 2,
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

// auto_fix_enabled/auto_merge_enabled are per-PR; pr_options mirrors them
// into one entry matching makeTestPR()'s identity (task-1/""/42) so overrides
// like `makeTestCIOptions({ auto_fix_enabled: true })` still drive the chip.
export function makeTestCIOptions(
  overrides: Partial<TaskCIAutomationOptions> = {},
): TaskCIAutomationOptions {
  const autoFix = overrides.auto_fix_enabled ?? false;
  const autoMerge = overrides.auto_merge_enabled ?? false;
  return {
    task_id: "task-1",
    auto_fix_enabled: autoFix,
    auto_merge_enabled: autoMerge,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "Default CI fix prompt",
    using_default_prompt: true,
    updated_at: "2026-06-18T10:00:00Z",
    auto_fix_max_rounds: 10,
    pr_states: [],
    pr_options: [
      {
        task_id: "task-1",
        repository_id: "",
        pr_number: 42,
        auto_fix_enabled: autoFix,
        auto_merge_enabled: autoMerge,
        prompt_on_review_requested: false,
        prompt_on_merged: false,
        prompt_on_closed: false,
        created_at: "",
        updated_at: "",
      },
    ],
    ...overrides,
  };
}

import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type {
  GitHubStatus,
  TaskCIAutomationOptions,
  TaskCIPRAutomationState,
} from "@/lib/types/github";
import { registerGitHubHandlers } from "./github";

const baseStatus: GitHubStatus = {
  authenticated: true,
  username: "octocat",
  auth_method: "pat",
  token_configured: true,
  required_scopes: ["repo"],
};

const DEFAULT_CI_AUTO_FIX_PROMPT = "Default prompt";
const TASK_ID = "task-1";
const INITIAL_UPDATED_AT = "2026-08-11T10:00:00Z";
const NEWER_UPDATED_AT = "2026-08-11T10:01:00Z";
const ACTIVE_WORKSPACE_ID = "workspace-b";
const FOREIGN_WORKSPACE_ID = "workspace-a";
const EXISTING_ASSOCIATION_ID = "association-b";

function seedTaskPRScope(store: ReturnType<typeof createAppStore>) {
  store.getState().setActiveWorkspace(ACTIVE_WORKSPACE_ID);
  store.getState().setTaskPRs(
    { [TASK_ID]: [{ id: EXISTING_ASSOCIATION_ID, task_id: TASK_ID } as never] },
    {
      workspaceId: ACTIVE_WORKSPACE_ID,
      workspaceContextGeneration: store.getState().workspaceContextGeneration,
    },
  );
}

describe("registerGitHubHandlers CI options", () => {
  it("does not let a delayed CI-options event replace newer state", () => {
    const store = createAppStore();
    const handler = registerGitHubHandlers(store)["github.task_ci_options.updated"]!;
    const options = (
      updatedAt: string,
      enabled: boolean,
      prStates: TaskCIPRAutomationState[] = [],
    ): TaskCIAutomationOptions => ({
      task_id: TASK_ID,
      auto_fix_enabled: enabled,
      auto_merge_enabled: false,
      auto_fix_prompt_override: null,
      effective_auto_fix_prompt: DEFAULT_CI_AUTO_FIX_PROMPT,
      using_default_prompt: true,
      updated_at: updatedAt,
      pr_states: prStates,
      pr_options: [],
    });

    handler({ payload: options(NEWER_UPDATED_AT, true) } as Parameters<typeof handler>[0]);
    handler({ payload: options(INITIAL_UPDATED_AT, false) } as Parameters<typeof handler>[0]);

    expect(store.getState().taskCIAutomation.byTaskId[TASK_ID]?.auto_fix_enabled).toBe(true);
  });

  it("applies a newer CI state event when task options are unchanged", () => {
    const store = createAppStore();
    const handler = registerGitHubHandlers(store)["github.task_ci_options.updated"]!;
    const prState: TaskCIPRAutomationState = {
      task_id: TASK_ID,
      repository_id: "repo-1",
      pr_number: 42,
      last_fix_signature: "check-failure",
      last_fix_checkpoint_json: "{}",
      last_fix_enqueued_at: NEWER_UPDATED_AT,
      last_fix_session_id: "session-1",
      auto_fix_round_count: 1,
      auto_fix_exhausted_at: null,
      last_merge_signature: "",
      last_merge_attempt_at: null,
      review_request_initialized: false,
      last_review_requested: false,
      last_observed_pr_state: "open",
      last_lifecycle_event: "",
      last_lifecycle_prompt_at: null,
      last_lifecycle_session_id: null,
      last_error: "Tests are failing",
      created_at: NEWER_UPDATED_AT,
      updated_at: NEWER_UPDATED_AT,
    };
    const initial: TaskCIAutomationOptions = {
      task_id: TASK_ID,
      auto_fix_enabled: true,
      auto_merge_enabled: false,
      auto_fix_prompt_override: null,
      effective_auto_fix_prompt: DEFAULT_CI_AUTO_FIX_PROMPT,
      using_default_prompt: true,
      updated_at: INITIAL_UPDATED_AT,
      pr_states: [],
      pr_options: [],
    };
    const updated: TaskCIAutomationOptions = {
      ...initial,
      updated_at: NEWER_UPDATED_AT,
      pr_states: [prState],
    };

    handler({ payload: initial } as Parameters<typeof handler>[0]);
    handler({ payload: updated } as Parameters<typeof handler>[0]);

    expect(store.getState().taskCIAutomation.byTaskId[TASK_ID]).toEqual(updated);
  });
});

describe("registerGitHubHandlers", () => {
  it("ignores a task PR update owned by another workspace", () => {
    const store = createAppStore();
    seedTaskPRScope(store);

    const handler = registerGitHubHandlers(store)["github.task_pr.updated"]!;
    handler({
      payload: {
        id: "association-a",
        task_id: TASK_ID,
        workspace_id: FOREIGN_WORKSPACE_ID,
      },
    } as Parameters<typeof handler>[0]);

    expect(store.getState().taskPRs.byTaskId[TASK_ID]?.map((pr) => pr.id)).toEqual([
      EXISTING_ASSOCIATION_ID,
    ]);
  });

  it("ignores an unattributed task PR update", () => {
    const store = createAppStore();
    seedTaskPRScope(store);

    const handler = registerGitHubHandlers(store)["github.task_pr.updated"]!;
    handler({
      payload: {
        id: "association-without-workspace",
        task_id: TASK_ID,
      },
    } as Parameters<typeof handler>[0]);

    expect(store.getState().taskPRs.byTaskId[TASK_ID]?.map((pr) => pr.id)).toEqual([
      EXISTING_ASSOCIATION_ID,
    ]);
  });

  it("removes the detached PR association without touching sibling PRs", () => {
    const store = createAppStore();
    const first = {
      id: "association-1",
      task_id: TASK_ID,
      owner: "acme",
      repo: "kandev",
      pr_number: 1,
    };
    const sibling = { ...first, id: "association-2", pr_number: 2 };
    store.getState().setTaskPRs({ [TASK_ID]: [first as never, sibling as never] });

    const handler = registerGitHubHandlers(store)["github.task_pr.deleted"]!;
    handler({
      payload: {
        workspace_id: "workspace-1",
        task_id: TASK_ID,
        association_id: "association-1",
      },
    } as Parameters<typeof handler>[0]);

    expect(store.getState().taskPRs.byTaskId[TASK_ID]?.map((pr) => pr.id)).toEqual([
      "association-2",
    ]);
  });

  it("applies unscoped rate-limit events only to legacy shared connections", () => {
    const store = createAppStore();
    store.getState().resetGitHubStatus("legacy-workspace");
    store.getState().setGitHubStatus("legacy-workspace", {
      ...baseStatus,
      automation: {
        workspace_id: "legacy-workspace",
        source: "legacy_shared",
        github_host: "github.com",
        status: "active",
        credential_generation: 1,
      },
    });
    store.getState().resetGitHubStatus("pat-workspace");
    store.getState().setGitHubStatus("pat-workspace", { ...baseStatus });

    const handler = registerGitHubHandlers(store)["github.rate_limit.updated"]!;
    handler({
      payload: {
        trigger: "core",
        snapshots: [
          {
            resource: "core",
            remaining: 0,
            limit: 5000,
            reset_at: "2030-01-01T00:00:00Z",
            updated_at: "2026-05-04T12:00:00Z",
          },
        ],
      },
    } as Parameters<typeof handler>[0]);

    expect(
      store.getState().githubStatus.byWorkspaceId["legacy-workspace"]?.status?.rate_limit?.core
        ?.remaining,
    ).toBe(0);
    expect(
      store.getState().githubStatus.byWorkspaceId["pat-workspace"]?.status?.rate_limit,
    ).toBeUndefined();
  });
});

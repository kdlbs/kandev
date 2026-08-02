import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { GitHubStatus } from "@/lib/types/github";
import { registerGitHubHandlers } from "./github";

const baseStatus: GitHubStatus = {
  authenticated: true,
  username: "octocat",
  auth_method: "pat",
  token_configured: true,
  required_scopes: ["repo"],
};

describe("registerGitHubHandlers", () => {
  it("removes the detached PR association without touching sibling PRs", () => {
    const store = createAppStore();
    const first = {
      id: "association-1",
      task_id: "task-1",
      owner: "acme",
      repo: "kandev",
      pr_number: 1,
    };
    const sibling = { ...first, id: "association-2", pr_number: 2 };
    store.getState().setTaskPRs({ "task-1": [first as never, sibling as never] });

    const handler = registerGitHubHandlers(store)["github.task_pr.deleted"]!;
    handler({
      payload: {
        workspace_id: "workspace-1",
        task_id: "task-1",
        association_id: "association-1",
      },
    } as Parameters<typeof handler>[0]);

    expect(store.getState().taskPRs.byTaskId["task-1"]?.map((pr) => pr.id)).toEqual([
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

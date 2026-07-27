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

import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

/**
 * Fixtures for the sidebar PR background-refresh specs. Two tasks are enough:
 * one keeps the route active, the other stays unopened so its row can only be
 * updated by the shared task-status-summary projection.
 */
export const PR_SIDEBAR_REFRESH = {
  navigationTitle: "PR background refresh navigation",
  targetTitle: "PR background refresh target",
  owner: "kandev-e2e",
  repo: "sidebar-background-refresh",
  prNumber: 4733,
  headBranch: "feature/background-refresh",
} as const;

export type PRSidebarRefreshSeed = {
  navigationTaskId: string;
  targetTaskId: string;
};

export type PRSidebarRefreshState = "open" | "merged";

type GatewayFrame = {
  action?: string;
  payload?: Record<string, unknown>;
};

/** Record outbound gateway frames so a spec can reject an unneeded sync. */
export function recordGatewayFrames(page: Page) {
  const sent: GatewayFrame[] = [];
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", (event) => {
      const text =
        typeof event.payload === "string"
          ? event.payload
          : new TextDecoder("utf-8", { fatal: false }).decode(event.payload);
      try {
        const frame = JSON.parse(text) as GatewayFrame;
        if (frame && typeof frame === "object") sent.push(frame);
      } catch {
        // Terminal and preview sockets carry binary traffic; ignore it.
      }
    });
  });
  return {
    reset(): void {
      sent.length = 0;
    },
    syncRequestsFor(taskId: string): GatewayFrame[] {
      return sent.filter(
        (frame) => frame.action === "github.task_pr.sync" && frame.payload?.task_id === taskId,
      );
    },
  };
}

function prPayload(targetTaskId: string, state: PRSidebarRefreshState) {
  const merged = state === "merged";
  return {
    task_id: targetTaskId,
    owner: PR_SIDEBAR_REFRESH.owner,
    repo: PR_SIDEBAR_REFRESH.repo,
    pr_number: PR_SIDEBAR_REFRESH.prNumber,
    pr_url: `https://github.test/${PR_SIDEBAR_REFRESH.owner}/${PR_SIDEBAR_REFRESH.repo}/pull/${PR_SIDEBAR_REFRESH.prNumber}`,
    pr_title: "Sidebar background refresh PR",
    head_branch: PR_SIDEBAR_REFRESH.headBranch,
    base_branch: "main",
    author_login: "test-user",
    state,
    review_state: "approved",
    checks_state: merged ? "success" : "pending",
    mergeable_state: merged ? "clean" : "blocked",
    checks_total: 2,
    checks_passing: merged ? 2 : 1,
  } as const;
}

export async function seedPRSidebarBackgroundRefresh(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<PRSidebarRefreshSeed> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const navigation = await apiClient.seedTask(
    seedData.workspaceId,
    PR_SIDEBAR_REFRESH.navigationTitle,
    stepOptions,
  );
  const target = await apiClient.seedTask(
    seedData.workspaceId,
    PR_SIDEBAR_REFRESH.targetTitle,
    stepOptions,
  );
  for (const taskId of [navigation.task_id, target.task_id]) {
    await apiClient.seedTaskSession(taskId, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
  }
  await apiClient.mockGitHubAssociateTaskPR({
    workspace_id: seedData.workspaceId,
    repository_id: seedData.repositoryId,
    ...prPayload(target.task_id, "open"),
  });
  // Seed the underlying feedback as well as the aggregate, so a feedback read
  // cannot change eligibility while the row is asserted.
  await apiClient.mockGitHubSeedPRFeedback({
    owner: PR_SIDEBAR_REFRESH.owner,
    repo: PR_SIDEBAR_REFRESH.repo,
    pr_number: PR_SIDEBAR_REFRESH.prNumber,
    checks: [{ name: "unit", status: "in_progress" }],
    comments: [],
    reviews: [],
  });
  return { navigationTaskId: navigation.task_id, targetTaskId: target.task_id };
}

/** Publish a newer provider status for the unopened target task. */
export async function publishPRSidebarBackgroundRefresh(
  apiClient: ApiClient,
  seedData: SeedData,
  seed: PRSidebarRefreshSeed,
  state: PRSidebarRefreshState,
): Promise<void> {
  await apiClient.mockGitHubAssociateTaskPR({
    workspace_id: seedData.workspaceId,
    repository_id: seedData.repositoryId,
    ...prPayload(seed.targetTaskId, state),
  });
}

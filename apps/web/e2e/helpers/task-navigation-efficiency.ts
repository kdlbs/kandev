import type { Page, Route } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

export function createNavigationTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

export const NAVIGATION_PR_OWNER = "kandev-e2e";
export const NAVIGATION_PR_REPO = "task-switch-feedback";

export async function associateNavigationPR(
  apiClient: ApiClient,
  workspaceId: string,
  taskId: string,
  prNumber: number,
) {
  await apiClient.mockGitHubAssociateTaskPR({
    workspace_id: workspaceId,
    task_id: taskId,
    owner: NAVIGATION_PR_OWNER,
    repo: NAVIGATION_PR_REPO,
    pr_number: prNumber,
    pr_url: `https://github.com/${NAVIGATION_PR_OWNER}/${NAVIGATION_PR_REPO}/pull/${prNumber}`,
    pr_title: `Task switch PR ${prNumber}`,
    head_branch: `feature/task-switch-${prNumber}`,
    base_branch: "main",
    author_login: "test-user",
    state: "open",
    checks_state: "failure",
    checks_total: 1,
    checks_passing: 0,
  });
  await apiClient.mockGitHubSeedPRFeedback({
    owner: NAVIGATION_PR_OWNER,
    repo: NAVIGATION_PR_REPO,
    pr_number: prNumber,
    checks: [{ name: "task-switch-ci", status: "completed", conclusion: "failure" }],
  });
}

export type HeldPRFeedbackRead = {
  requestStarted: Promise<void>;
  requestCount: () => number;
  release: () => void;
  dispose: () => Promise<void>;
};

export async function holdPRFeedbackRead(
  page: Page,
  workspaceId: string,
  owner: string,
  repo: string,
  prNumber: number,
): Promise<HeldPRFeedbackRead> {
  const urlPattern = `**/api/v1/github/prs/${owner}/${repo}/${prNumber}*`;
  let releaseResponse!: () => void;
  let notifyRequestStarted!: () => void;
  let held = false;
  let requestCount = 0;
  const released = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });
  const requestStarted = new Promise<void>((resolve) => {
    notifyRequestStarted = resolve;
  });
  const handler = async (route: Route) => {
    const requestUrl = new URL(route.request().url());
    if (requestUrl.searchParams.get("workspace_id") !== workspaceId) {
      await route.continue();
      return;
    }
    requestCount++;
    if (held) {
      await route.continue();
      return;
    }
    held = true;
    const response = await route.fetch();
    notifyRequestStarted();
    await released;
    await route.fulfill({ response });
  };
  await page.route(urlPattern, handler);

  return {
    requestStarted,
    requestCount: () => requestCount,
    release: releaseResponse,
    dispose: async () => {
      releaseResponse();
      await page.unroute(urlPattern, handler);
    },
  };
}

export type HeldTaskEnrichmentRead = {
  requestStarted: Promise<void>;
  release: () => void;
  dispose: () => Promise<void>;
};

export type HeldAgentListRead = {
  requestStarted: Promise<void>;
  requestCount: () => number;
  release: () => void;
  dispose: () => Promise<void>;
};

export type HeldIntegrationHealthRead = {
  requestStarted: Promise<void>;
  requestCount: () => number;
  release: () => void;
  dispose: () => Promise<void>;
};

export async function holdIntegrationHealthRead(
  page: Page,
  pathname: string,
  workspaceId: string,
): Promise<HeldIntegrationHealthRead> {
  let releaseResponse!: () => void;
  let notifyRequestStarted!: () => void;
  let held = false;
  let requestCount = 0;
  const released = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });
  const requestStarted = new Promise<void>((resolve) => {
    notifyRequestStarted = resolve;
  });
  const matchesWorkspaceRead = (url: URL) => url.pathname === pathname;
  const handler = async (route: Route) => {
    const url = new URL(route.request().url());
    if (
      !matchesWorkspaceRead(url) ||
      url.searchParams.get("workspace_id") !== workspaceId ||
      route.request().method() !== "GET"
    ) {
      await route.continue();
      return;
    }
    requestCount++;
    if (held) {
      await route.continue();
      return;
    }
    held = true;
    const response = await route.fetch();
    notifyRequestStarted();
    await released;
    await route.fulfill({ response });
  };
  const matcher = (url: URL) => matchesWorkspaceRead(url);
  await page.route(matcher, handler);

  return {
    requestStarted,
    requestCount: () => requestCount,
    release: releaseResponse,
    dispose: async () => {
      releaseResponse();
      await page.unroute(matcher, handler);
    },
  };
}

export async function holdAgentListRead(page: Page): Promise<HeldAgentListRead> {
  let releaseResponse!: () => void;
  let notifyRequestStarted!: () => void;
  let held = false;
  let requestCount = 0;
  const released = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });
  const requestStarted = new Promise<void>((resolve) => {
    notifyRequestStarted = resolve;
  });
  const matchesAgentList = (url: URL) => url.pathname === "/api/v1/agents";
  const handler = async (route: Route) => {
    if (!matchesAgentList(new URL(route.request().url()))) {
      await route.continue();
      return;
    }
    requestCount++;
    if (held) {
      await route.continue();
      return;
    }
    held = true;
    const response = await route.fetch();
    notifyRequestStarted();
    await released;
    await route.fulfill({ response });
  };
  await page.route((url) => matchesAgentList(url), handler);

  return {
    requestStarted,
    requestCount: () => requestCount,
    release: releaseResponse,
    dispose: async () => {
      releaseResponse();
      await page.unroute((url) => matchesAgentList(url), handler);
    },
  };
}

export async function holdTaskEnrichmentRead(
  page: Page,
  sessionId: string,
): Promise<HeldTaskEnrichmentRead> {
  const urlPattern = `**/api/v1/task-sessions/${sessionId}`;
  let releaseResponse!: () => void;
  let notifyRequestStarted!: () => void;
  let held = false;
  const released = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });
  const requestStarted = new Promise<void>((resolve) => {
    notifyRequestStarted = resolve;
  });
  const handler = async (route: Route) => {
    if (held || route.request().headers()["if-none-match"]) {
      await route.continue();
      return;
    }
    held = true;
    notifyRequestStarted();
    await released;
    await route.continue();
  };
  await page.route(urlPattern, handler);

  return {
    requestStarted,
    release: releaseResponse,
    dispose: async () => {
      releaseResponse();
      await page.unroute(urlPattern, handler);
    },
  };
}

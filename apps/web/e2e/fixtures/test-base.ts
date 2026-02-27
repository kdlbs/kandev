import { type Page } from "@playwright/test";
import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { backendFixture, type BackendContext } from "./backend";
import { ApiClient } from "../helpers/api-client";
import type { WorkflowStep } from "../../lib/types/http";

export type SeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  steps: WorkflowStep[];
  repositoryId: string;
};

export const test = backendFixture.extend<
  { testPage: Page },
  { apiClient: ApiClient; seedData: SeedData }
>({
  // Worker-scoped API client
  apiClient: [
    async ({ backend }, use) => {
      const client = new ApiClient(backend.baseUrl);
      await use(client);
    },
    { scope: "worker" },
  ],

  // Worker-scoped seed data: creates workspace, workflow (from template), discovers steps,
  // and sets up a local git repository for agent execution workspace.
  // The repo is created inside backend.tmpDir (the backend's HOME) so that
  // discoveryRoots() allows branch listing (isPathAllowed check).
  seedData: [
    async ({ apiClient, backend }, use) => {
      const workspace = await apiClient.createWorkspace("E2E Workspace");
      const workflow = await apiClient.createWorkflow(workspace.id, "E2E Workflow", "simple");

      const { steps } = await apiClient.listWorkflowSteps(workflow.id);
      const sorted = steps.sort((a, b) => a.position - b.position);
      const startStep = sorted.find((s) => s.is_start_step) ?? sorted[0];

      // Create a minimal git repository inside backend.tmpDir (the backend's HOME).
      // This ensures discoveryRoots() allows the path for branch listing.
      const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
      fs.mkdirSync(repoDir, { recursive: true });
      const gitEnv = {
        ...process.env,
        HOME: backend.tmpDir,
        GIT_AUTHOR_NAME: "E2E Test",
        GIT_AUTHOR_EMAIL: "e2e@test.local",
        GIT_COMMITTER_NAME: "E2E Test",
        GIT_COMMITTER_EMAIL: "e2e@test.local",
      };
      execSync("git init -b main", { cwd: repoDir, env: gitEnv });
      execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env: gitEnv });
      const repo = await apiClient.createRepository(workspace.id, repoDir);

      await use({
        workspaceId: workspace.id,
        workflowId: workflow.id,
        startStepId: startStep.id,
        steps: sorted,
        repositoryId: repo.id,
      });
    },
    { scope: "worker" },
  ],

  // Per-test page with baseURL pointing to worker's frontend.
  // Resets user settings to the E2E workspace/workflow before each test so that
  // SSR always resolves to the correct workspace regardless of what commitSettings
  // may have written during previous tests.
  testPage: async ({ browser, backend, apiClient, seedData }, use) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
    });
    const context = await browser.newContext({
      baseURL: backend.frontendUrl,
    });
    const page = await context.newPage();
    await setupPage(page, backend);
    await use(page);
    await context.close();
  },
});

export { expect } from "@playwright/test";

async function setupPage(page: Page, backend: BackendContext): Promise<void> {
  await page.addInitScript(
    ({ backendUrl }: { backendUrl: string }) => {
      localStorage.setItem("kandev.onboarding.completed", "true");
      // Set the window global that getBackendConfig() reads for API/WS connections
      window.__KANDEV_API_BASE_URL = backendUrl;
    },
    { backendUrl: backend.baseUrl },
  );
}

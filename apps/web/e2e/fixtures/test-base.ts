import { type Page } from "@playwright/test";
import { backendFixture, type BackendContext } from "./backend";
import { ApiClient } from "../helpers/api-client";
import type { WorkflowStep } from "../../lib/types/http";

export type SeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  steps: WorkflowStep[];
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

  // Worker-scoped seed data: creates workspace, workflow (from template), and discovers steps
  seedData: [
    async ({ apiClient }, use) => {
      const workspace = await apiClient.createWorkspace("E2E Workspace");
      const workflow = await apiClient.createWorkflow(workspace.id, "E2E Workflow", "simple");

      const { steps } = await apiClient.listWorkflowSteps(workflow.id);
      const sorted = steps.sort((a, b) => a.position - b.position);
      const startStep = sorted.find((s) => s.is_start_step) ?? sorted[0];

      await use({
        workspaceId: workspace.id,
        workflowId: workflow.id,
        startStepId: startStep.id,
        steps: sorted,
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

import { type Page } from "@playwright/test";
import { test as base } from "./test-base";
import { OfficeApiClient } from "../helpers/office-api-client";

type OfficeFixtures = {
  officeApi: OfficeApiClient;
  officeSeed: {
    workspaceId: string;
    agentId: string;
    projectId: string;
    workflowId: string;
  };
};

export const test = base.extend<{ testPage: Page }, OfficeFixtures>({
  // Worker-scoped: create office API client pointing at the worker's backend.
  officeApi: [
    async ({ backend }, use) => {
      const client = new OfficeApiClient(backend.baseUrl);
      await use(client);
    },
    { scope: "worker" },
  ],

  // Worker-scoped: complete onboarding once per worker and expose the
  // resulting workspace/agent/project IDs to all tests in the suite.
  officeSeed: [
    async ({ officeApi, seedData }, use) => {
      const result = await officeApi.completeOnboarding({
        workspaceName: "E2E Workspace",
        taskPrefix: "E2E",
        agentName: "CEO",
        agentProfileId: seedData.agentProfileId,
        executorPreference: "local_pc",
      });
      await use({
        workspaceId: result.workspaceId,
        agentId: result.agentId,
        projectId: result.projectId,
        workflowId: seedData.workflowId,
      });
    },
    { scope: "worker" },
  ],

  // Override testPage to set the active workspace to officeSeed.workspaceId
  // so that office UI pages render with the seeded office data.
  //
  // The base test-base.ts testPage runs e2eReset against seedData.workspaceId,
  // but onboarding allocates its OWN workspace ID (officeSeed.workspaceId),
  // so per-test office task / session leftovers leak across tests unless we
  // reset the office workspace here as well.
  testPage: async ({ testPage: basePage, apiClient, officeSeed, seedData }, use) => {
    if (officeSeed.workspaceId !== seedData.workspaceId) {
      await apiClient.e2eReset(officeSeed.workspaceId, [
        seedData.workflowId,
        officeSeed.workflowId,
      ]);
    }
    await apiClient.saveUserSettings({
      workspace_id: officeSeed.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });
    await use(basePage);
  },
});

export { expect } from "@playwright/test";

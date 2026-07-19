import { type Page } from "@playwright/test";
import { test as base, expect } from "../../fixtures/test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";

/**
 * Regression test: onboarding with a task title must launch the agent
 * successfully even when no repository is associated with the task.
 *
 * Before the fix, the scheduler failed with "workspace_path is required"
 * because the lifecycle manager only created fallback workspace directories
 * for ephemeral (quick-chat) tasks, not for office tasks.
 */

type OnboardingTaskFixtures = {
  officeApi: OfficeApiClient;
  onboardingSeed: {
    workspaceId: string;
    agentId: string;
    taskId: string;
  };
};

const test = base.extend<{ testPage: Page }, OnboardingTaskFixtures>({
  officeApi: [
    async ({ backend }, use) => {
      await use(new OfficeApiClient(backend.baseUrl));
    },
    { scope: "worker" },
  ],

  // Complete onboarding WITH a task title — this is the flow that was broken.
  onboardingSeed: [
    async ({ officeApi, seedData }, use) => {
      const result = (await officeApi.completeOnboarding({
        workspaceName: "Task Launch Workspace",
        taskPrefix: "TL",
        agentName: "CEO",
        agentProfileId: seedData.agentProfileId,
        executorPreference: "local_pc",
        taskTitle: "Present yourself",
        taskDescription: "Introduce yourself to the team",
      })) as { workspaceId: string; agentId: string; projectId: string; taskId?: string };

      if (!result.taskId) {
        throw new Error("completeOnboarding did not return a taskId");
      }

      await use({
        workspaceId: result.workspaceId,
        agentId: result.agentId,
        taskId: result.taskId,
      });
    },
    { scope: "worker" },
  ],

  testPage: async ({ testPage: basePage, apiClient, onboardingSeed, seedData }, use) => {
    await apiClient.saveUserSettings({
      workspace_id: onboardingSeed.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });
    await use(basePage);
  },
});

test.describe("Onboarding task launch", () => {
  test("task created during onboarding does not fail to launch", async ({
    apiClient,
    onboardingSeed,
  }) => {
    test.setTimeout(30_000);

    // Office task status is workflow-owned and intentionally stays CREATED
    // until the agent changes it. Poll the session to prove runtime launch.
    let lastState = "";
    const deadline = Date.now() + 15_000;

    while (Date.now() < deadline) {
      const { sessions } = await apiClient.listTaskSessions(onboardingSeed.taskId);
      lastState = sessions[0]?.state ?? "";

      expect(lastState, "session should not terminate before launch").not.toMatch(
        /^(FAILED|CANCELLED)$/,
      );

      if (["RUNNING", "WAITING_FOR_INPUT", "IDLE", "COMPLETED"].includes(lastState)) {
        return;
      }

      await new Promise((r) => setTimeout(r, 1_000));
    }

    expect(
      ["RUNNING", "WAITING_FOR_INPUT", "IDLE", "COMPLETED"],
      `expected an agent session to launch, got "${lastState}"`,
    ).toContain(lastState);
  });
});

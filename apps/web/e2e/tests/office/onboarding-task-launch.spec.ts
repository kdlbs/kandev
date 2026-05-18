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
    officeApi,
    onboardingSeed,
  }) => {
    test.setTimeout(30_000);

    // Poll the task state — it should progress past CREATED/SCHEDULING
    // and NOT end up in FAILED. The scheduler picks up the run within
    // ~5 seconds, so we give it up to 15 seconds.
    let lastState = "";
    const deadline = Date.now() + 15_000;

    while (Date.now() < deadline) {
      const issue = await officeApi.getTask(onboardingSeed.taskId);
      const raw = issue as Record<string, unknown>;
      const inner = (raw.task as Record<string, unknown>) ?? raw;
      lastState = (inner.state as string) ?? (inner.status as string) ?? "";

      // FAILED means the agent launch failed — this is the regression.
      // Status comes back as the canonical office lowercase form via
      // dbStateToOfficeStatus (in_progress, in_review, done, …); legacy
      // SCREAMING_SNAKE_CASE values are accepted for forward compatibility.
      expect(lastState, "task should not enter FAILED state").not.toMatch(/^(failed|FAILED)$/);

      // Any of these states means the agent launched successfully.
      const launched = ["in_progress", "scheduling", "in_review", "done", "completed"];
      if (launched.includes(lastState.toLowerCase())) {
        return; // Success — agent launched without workspace_path error
      }

      await new Promise((r) => setTimeout(r, 1_000));
    }

    // If we're still in CREATED after 15s, the scheduler didn't pick it up.
    // That's a different issue, but not the regression we're testing.
    expect(
      ["in_progress", "scheduling", "in_review", "done", "completed"],
      `expected task to be scheduled, got "${lastState}"`,
    ).toContain(lastState.toLowerCase());
  });
});

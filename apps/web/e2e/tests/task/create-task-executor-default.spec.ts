import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

// Exercises the regular task-create dialog (New Task in the sidebar), so run
// with the office feature disabled.
useRegularMode();

type ExecutorProfile = { id: string; name: string };

type ExecutorSummary = {
  id: string;
  type: string;
  profiles?: ExecutorProfile[];
};

async function executorProfiles(apiClient: {
  listExecutors: () => Promise<{ executors: ExecutorSummary[] }>;
}) {
  const { executors } = await apiClient.listExecutors();
  const localExecutor = executors.find((executor) => ["local", "local_pc"].includes(executor.type));
  const worktreeExecutor = executors.find((executor) => executor.type === "worktree");
  const localProfile = localExecutor?.profiles?.[0];
  const worktreeProfile = worktreeExecutor?.profiles?.[0];
  expect(localProfile, "a direct local executor profile is required by the fixture").toBeDefined();
  expect(worktreeProfile, "a worktree executor profile is required by the fixture").toBeDefined();
  return { localExecutor, localProfile: localProfile!, worktreeProfile: worktreeProfile! };
}

async function saveTaskCreatePreference(
  apiClient: {
    saveUserSettings: (settings: {
      workspace_id: string;
      workflow_filter_id: string;
      task_create_last_used: {
        repository_id: string;
        branch: string;
        agent_profile_id: string;
        executor_profile_id: string;
      };
    }) => Promise<void>;
  },
  seedData: {
    workspaceId: string;
    workflowId: string;
    repositoryId: string;
    agentProfileId: string;
  },
  executorProfileId: string,
) {
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    workflow_filter_id: seedData.workflowId,
    task_create_last_used: {
      repository_id: seedData.repositoryId,
      branch: "main",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: executorProfileId,
    },
  });
}

async function openCreateTask(testPage: import("@playwright/test").Page) {
  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  await kanban.createTaskButton.first().click();
  await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
}

test.describe("Task-create executor safety defaults", () => {
  test("executor safety default ignores a saved local profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { localProfile, worktreeProfile } = await executorProfiles(apiClient);
    await apiClient.updateWorkspace(seedData.workspaceId, { default_executor_id: "" });
    await saveTaskCreatePreference(apiClient, seedData, localProfile.id);

    try {
      await openCreateTask(testPage);
      await expect(testPage.getByTestId("executor-profile-selector")).toContainText(
        worktreeProfile.name,
      );
    } finally {
      await apiClient.updateWorkspace(seedData.workspaceId, { default_executor_id: "" });
      await saveTaskCreatePreference(apiClient, seedData, worktreeProfile.id);
    }
  });

  test("executor safety default honors an explicit local workspace default", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { localExecutor, localProfile, worktreeProfile } = await executorProfiles(apiClient);
    expect(localExecutor).toBeDefined();
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_executor_id: localExecutor!.id,
    });
    await saveTaskCreatePreference(apiClient, seedData, worktreeProfile.id);

    try {
      await openCreateTask(testPage);
      await expect(testPage.getByTestId("executor-profile-selector")).toContainText(
        localProfile.name,
      );
    } finally {
      await apiClient.updateWorkspace(seedData.workspaceId, { default_executor_id: "" });
      await saveTaskCreatePreference(apiClient, seedData, worktreeProfile.id);
    }
  });
});

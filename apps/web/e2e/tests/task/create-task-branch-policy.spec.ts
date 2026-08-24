import { execSync } from "node:child_process";
import { expect, test } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { useRegularMode } from "../../helpers/regular-mode";

useRegularMode();

test.describe("Task creation with branch policies", () => {
  test("selects a policy, enables fresh branch mode, and snapshots it", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    execSync("git branch -f develop", {
      cwd: seedData.repositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });
    const policy = await apiClient.createRepositoryBranchPolicy(seedData.repositoryId, {
      name: `Feature policy ${Date.now()}`,
      description: "Task creation policy",
      base_branch: "main",
      branch_template: "feature/{title}-{suffix}",
      pull_request_target: "develop",
    });
    const { executors } = await apiClient.listExecutors();
    const localExecutor = executors.find((executor) => executor.type === "local");
    if (!localExecutor) {
      test.skip(true, "No local executor available");
      return;
    }
    const localProfile = await apiClient.createExecutorProfile(
      localExecutor.id,
      `E2E Branch Policy Local ${Date.now()}`,
    );

    try {
      await testPage.goto("/");
      await testPage.getByTestId("create-task-button").first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByTestId("executor-profile-selector").click();
      await testPage.getByRole("option", { name: new RegExp(localProfile.name) }).click();
      await dialog.getByTestId("branch-chip-trigger").click();
      const option = testPage.getByRole("option", { name: new RegExp(policy.name) });
      await expect(option).toContainText("Policy");
      await option.click();
      await expect(dialog.getByTestId("branch-chip-trigger")).toContainText(policy.name);
      await expect(dialog.getByTestId("fresh-branch-toggle")).toHaveAttribute(
        "aria-pressed",
        "true",
      );

      const title = `Policy task ${Date.now()}`;
      await dialog.getByTestId("task-title-input").fill(title);
      await dialog.getByTestId("task-description-input").fill("Create from a branch policy");
      await dialog.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      await expect(dialog).not.toBeVisible();

      let created: { id: string; title: string } | undefined;
      await expect
        .poll(async () => {
          const response = await apiClient.listTasks(seedData.workspaceId);
          created = response.tasks.find((task) => task.title === title);
          return created;
        })
        .toBeDefined();
      expect(created).toBeDefined();
      const taskResponse = await apiClient.rawRequest("GET", `/api/v1/tasks/${created!.id}`);
      const task = (await taskResponse.json()) as {
        repositories: Array<{
          branch_policy_id?: string;
          branch_policy_name?: string;
          branch_policy_base_branch?: string;
          branch_policy_branch_template?: string;
          branch_policy_pull_request_target?: string;
        }>;
      };
      expect(task.repositories[0]).toEqual(
        expect.objectContaining({
          branch_policy_id: policy.id,
          branch_policy_name: policy.name,
          branch_policy_base_branch: "main",
          branch_policy_branch_template: "feature/{title}-{suffix}",
          branch_policy_pull_request_target: "develop",
        }),
      );
      const currentBranch = execSync("git branch --show-current", {
        cwd: seedData.repositoryPath,
        env: makeGitEnv(backend.tmpDir),
      })
        .toString()
        .trim();
      expect(currentBranch).toMatch(/^feature\/policy-task-/);
    } finally {
      await apiClient.deleteExecutorProfile(localProfile.id).catch(() => {});
    }
  });
});

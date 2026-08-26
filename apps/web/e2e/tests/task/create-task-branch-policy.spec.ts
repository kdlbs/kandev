import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { useRegularMode } from "../../helpers/regular-mode";
import { expectPolicyOptionUsesOneLine } from "./create-task-branch-policy-helpers";

useRegularMode();

test.describe("Task creation with branch policies", () => {
  test("selects a policy, enables fresh branch mode, and snapshots it", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    execSync("git clean -fd", {
      cwd: seedData.repositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });
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
      await expect(dialog.getByTestId("executor-profile-selector")).toContainText(
        localProfile.name,
      );
      await dialog.getByTestId("branch-chip-trigger").click();
      const option = testPage.getByRole("option", { name: new RegExp(policy.name) });
      await expect(option).toContainText("Policy");
      await expectPolicyOptionUsesOneLine(option, policy.name);
      const policyInfo = testPage.getByTestId(`branch-policy-option-info-${policy.id}`);
      await policyInfo.hover();
      await expect(testPage.getByRole("tooltip")).toContainText(
        "Base: main. Template: feature/{title}-{suffix}. Pull request target: develop.",
      );
      await testPage.mouse.move(0, 0);
      await policyInfo.focus();
      await expect(policyInfo).toBeFocused();
      await expect(policyInfo).toHaveAttribute(
        "aria-label",
        "Base: main. Template: feature/{title}-{suffix}. Pull request target: develop.",
      );
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
      await expect(dialog).not.toBeVisible({ timeout: 30_000 });

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

  test("disables policy selection for a multi-repository local task", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const secondRepositoryPath = path.join(
      backend.tmpDir,
      "repos",
      `branch-policy-multi-repo-${Date.now()}`,
    );
    fs.mkdirSync(secondRepositoryPath, { recursive: true });
    execSync("git init -b main", {
      cwd: secondRepositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });
    execSync('git commit --allow-empty -m "init"', {
      cwd: secondRepositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });
    const secondRepositoryName = `Policy second repository ${Date.now()}`;
    await apiClient.createRepository(seedData.workspaceId, secondRepositoryPath, "main", {
      name: secondRepositoryName,
    });
    const policy = await apiClient.createRepositoryBranchPolicy(seedData.repositoryId, {
      name: `Multi-repo Feature ${Date.now()}`,
      base_branch: "main",
      branch_template: "feature/{title}-{suffix}",
      pull_request_target: "main",
    });
    const { executors } = await apiClient.listExecutors();
    const localExecutor = executors.find((executor) => executor.type === "local");
    if (!localExecutor) {
      test.skip(true, "No local executor available");
      return;
    }
    const localProfile = await apiClient.createExecutorProfile(
      localExecutor.id,
      `E2E Multi-repo Branch Policy Local ${Date.now()}`,
    );

    try {
      await testPage.goto("/");
      await testPage.getByTestId("create-task-button").first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByTestId("executor-profile-selector").click();
      await testPage.getByRole("option", { name: new RegExp(localProfile.name) }).click();
      await expect(dialog.getByTestId("executor-profile-selector")).toContainText(
        localProfile.name,
      );

      await dialog.getByTestId("add-repository").click();
      const repositoryChips = dialog.getByTestId("repo-chip-trigger");
      await expect(repositoryChips).toHaveCount(2);
      await repositoryChips.nth(1).click();
      await testPage.getByRole("option", { name: new RegExp(secondRepositoryName) }).click();
      await expect(repositoryChips.nth(1)).toContainText(secondRepositoryName);

      const branchChips = dialog.getByTestId("branch-chip-trigger");
      await expect(branchChips).toHaveCount(2);
      await branchChips.nth(0).click();
      const policyOption = testPage.getByRole("option", { name: new RegExp(policy.name) });
      await expect(policyOption).toHaveAttribute("aria-disabled", "true");
      await expect(testPage.getByTestId(`branch-policy-option-info-${policy.id}`)).toHaveAttribute(
        "aria-label",
        /single repository/,
      );
    } finally {
      await apiClient.deleteExecutorProfile(localProfile.id).catch(() => {});
    }
  });
});

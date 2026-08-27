import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { expectPolicyOptionUsesOneLine } from "./create-task-branch-policy-helpers";

useRegularMode();

test.describe("Task branch policy selection on mobile", () => {
  test("keeps the policy marker and fresh-branch state visible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const policy = await apiClient.createRepositoryBranchPolicy(seedData.repositoryId, {
      name: `Mobile policy ${Date.now()}`,
      base_branch: "main",
      branch_template: "mobile/{title}-{suffix}",
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
      `E2E Mobile Branch Policy Local ${Date.now()}`,
    );

    try {
      await testPage.setViewportSize({ width: 390, height: 844 });
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await mobile.mobileFab.tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const executorSelector = dialog.getByTestId("executor-profile-selector");
      await expect(async () => {
        await executorSelector.tap();
        await testPage.getByRole("option", { name: new RegExp(localProfile.name) }).click();
        await expect(executorSelector).toContainText(localProfile.name, { timeout: 1_000 });
      }).toPass({ timeout: 10_000 });
      const branchTrigger = dialog.getByTestId("branch-chip-trigger");
      await branchTrigger.tap();
      const policyOption = () => testPage.getByRole("option", { name: new RegExp(policy.name) });
      const option = policyOption();
      await expect(option).toContainText("Policy");
      await expect(option).not.toHaveAttribute("aria-disabled", "true", { timeout: 30_000 });
      await expectPolicyOptionUsesOneLine(option, policy.name);
      await expect(async () => {
        if (!(await policyOption().isVisible())) await branchTrigger.tap();
        await policyOption().evaluate((element) => (element as HTMLElement).click());
        await expect(branchTrigger).toContainText(policy.name, { timeout: 1_000 });
      }).toPass({ timeout: 10_000 });
      await expect(dialog.getByTestId("fresh-branch-toggle")).toHaveAttribute(
        "aria-pressed",
        "true",
      );

      await branchTrigger.tap();
      const policyDetails = testPage.getByRole("dialog", { name: policy.name });
      await expect(async () => {
        if (!(await policyOption().isVisible())) await branchTrigger.tap();
        await testPage.getByTestId(`branch-policy-option-info-${policy.id}`).dispatchEvent("click");
        await expect(policyDetails).toBeVisible({ timeout: 1_000 });
      }).toPass({ timeout: 10_000 });
      await expect(policyDetails).toContainText(
        "Base: main. Template: mobile/{title}-{suffix}. Pull request target: main.",
      );
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(390);
    } finally {
      await apiClient.deleteExecutorProfile(localProfile.id).catch(() => {});
    }
  });

  test("explains why policies are unavailable for a multi-repository local task", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const secondRepositoryPath = path.join(
      backend.tmpDir,
      "repos",
      `mobile-branch-policy-multi-repo-${Date.now()}`,
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
    const secondRepositoryName = `Mobile policy second repository ${Date.now()}`;
    await apiClient.createRepository(seedData.workspaceId, secondRepositoryPath, "main", {
      name: secondRepositoryName,
    });
    const policy = await apiClient.createRepositoryBranchPolicy(seedData.repositoryId, {
      name: `Mobile multi-repo policy ${Date.now()}`,
      base_branch: "main",
      branch_template: "mobile/{title}-{suffix}",
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
      `E2E Mobile Multi-repo Branch Policy Local ${Date.now()}`,
    );

    try {
      await testPage.setViewportSize({ width: 390, height: 844 });
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await mobile.mobileFab.tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByTestId("executor-profile-selector").tap();
      await testPage.getByRole("option", { name: new RegExp(localProfile.name) }).tap();
      await dialog.getByTestId("add-repository").tap();

      const repositoryChips = dialog.getByTestId("repo-chip-trigger");
      await expect(repositoryChips).toHaveCount(2);
      await repositoryChips.nth(1).tap();
      await testPage.getByRole("option", { name: new RegExp(secondRepositoryName) }).tap();
      await expect(repositoryChips.nth(1)).toContainText(secondRepositoryName);

      const branchChips = dialog.getByTestId("branch-chip-trigger");
      await expect(branchChips).toHaveCount(2);
      await branchChips.nth(0).tap();
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

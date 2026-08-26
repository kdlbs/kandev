import { execSync } from "node:child_process";
import { expect, test } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";

test.describe("Repository branch policies on desktop", () => {
  test("shows field help on hover and keyboard focus", async ({ testPage, seedData }) => {
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    const repositoryCard = testPage.locator('[data-slot="card"]', { hasText: "E2E Repo" });
    await repositoryCard.getByRole("button", { name: "Edit", exact: true }).click();

    const policies = testPage.getByTestId(`branch-policies-${seedData.repositoryId}`);
    await policies.locator("summary").click();
    await policies.getByRole("button", { name: "Add policy", exact: true }).click();

    const dialog = testPage.getByRole("dialog", { name: "Add branch policy" });
    const help = dialog.getByRole("button", { name: "About policy names", exact: true });
    await help.hover();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Use a short name that helps people choose this policy in the task dialog.",
    );

    await help.focus();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Use a short name that helps people choose this policy in the task dialog.",
    );
  });

  test("creates Gitflow policies, edits one, and deletes it", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    execSync("git branch -f develop", {
      cwd: seedData.repositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });
    execSync("git push origin HEAD:refs/heads/release-candidate", {
      cwd: seedData.repositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });
    execSync("git update-ref refs/remotes/origin/release-candidate HEAD", {
      cwd: seedData.repositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    const repositoryCard = testPage.locator('[data-slot="card"]', { hasText: "E2E Repo" });
    await repositoryCard.getByRole("button", { name: "Edit", exact: true }).click();

    const policies = testPage.getByTestId(`branch-policies-${seedData.repositoryId}`);
    await expect(policies.locator("summary")).toContainText("Branch policies");
    await policies.locator("summary").click();
    await expect(policies).toContainText("No branch policies yet.");
    await policies.getByRole("button", { name: "Add Gitflow policies", exact: true }).click();

    const gitflowDialog = testPage.getByRole("dialog", { name: "Add Gitflow policies" });
    await expect(gitflowDialog.getByRole("combobox", { name: "Production branch" })).toContainText(
      "main",
    );
    await expect(gitflowDialog.getByRole("combobox", { name: "Development branch" })).toContainText(
      "develop",
    );
    await gitflowDialog.getByRole("combobox", { name: "Development branch" }).click();
    await expect(gitflowDialog.getByPlaceholder("Search branches...")).toBeVisible();
    await expect(gitflowDialog.getByRole("option", { name: /^main local/ })).toBeVisible();
    await expect(
      gitflowDialog.getByRole("option", { name: /^origin\/release-candidate origin/ }),
    ).toBeVisible();
    await gitflowDialog.getByPlaceholder("Search branches...").fill("release-candidate");
    await expect(gitflowDialog.getByRole("option", { name: /^main local/ })).toHaveCount(0);
    await expect(
      gitflowDialog.getByRole("option", { name: /^origin\/release-candidate origin/ }),
    ).toBeVisible();
    await gitflowDialog.getByTestId("branch-refresh-button").click();
    await expect(gitflowDialog.getByTestId("branch-refresh-button")).toBeEnabled();
    await gitflowDialog.getByRole("option", { name: /^origin\/release-candidate origin/ }).click();
    await expect(gitflowDialog.getByRole("combobox", { name: "Development branch" })).toContainText(
      "origin/release-candidate",
    );
    await gitflowDialog.getByRole("combobox", { name: "Development branch" }).click();
    await gitflowDialog.getByPlaceholder("Search branches...").fill("develop");
    await gitflowDialog.getByRole("option", { name: /^develop local/ }).click();
    await gitflowDialog
      .getByRole("button", { name: "Create Gitflow policies", exact: true })
      .click();

    await expect
      .poll(async () => {
        const response = await apiClient.listRepositoryBranchPolicies(seedData.repositoryId);
        return response.repository_branch_policies.map((policy) => policy.name).sort();
      })
      .toEqual(["Bugfix", "Feature", "Hotfix", "Release"]);

    const feature = (
      await apiClient.listRepositoryBranchPolicies(seedData.repositoryId)
    ).repository_branch_policies.find((policy) => policy.name === "Feature");
    expect(feature).toBeDefined();
    const featureRow = testPage.getByTestId(`branch-policy-${feature!.id}`);
    await featureRow.getByRole("button", { name: "Edit Feature", exact: true }).click();
    const editDialog = testPage.getByRole("dialog", { name: "Edit branch policy" });
    await editDialog.getByLabel("Description (optional)").fill("Feature work");
    await editDialog.getByRole("button", { name: "Save", exact: true }).click();
    await expect
      .poll(async () => {
        const response = await apiClient.listRepositoryBranchPolicies(seedData.repositoryId);
        return response.repository_branch_policies.find((policy) => policy.id === feature!.id)
          ?.description;
      })
      .toBe("Feature work");

    await testPage
      .getByTestId(`branch-policy-${feature!.id}`)
      .getByRole("button", { name: "Delete Feature", exact: true })
      .click();
    const deleteDialog = testPage.getByRole("alertdialog");
    await expect(deleteDialog).toContainText("Existing tasks keep their saved policy snapshot");
    await deleteDialog.getByRole("button", { name: "Delete policy", exact: true }).click();
    await expect
      .poll(async () => {
        const response = await apiClient.listRepositoryBranchPolicies(seedData.repositoryId);
        return response.repository_branch_policies.some((policy) => policy.id === feature!.id);
      })
      .toBe(false);
  });
});

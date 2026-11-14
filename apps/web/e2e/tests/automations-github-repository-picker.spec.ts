import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";
import { expectControlHeight } from "../helpers/control-sizing";
import {
  ACCESSIBLE_REPOSITORIES,
  expandPrTriggerCard,
  openRestrictedPrAutomation,
  organizationOptions,
  repositoryOptions,
  seedAccessibleRepositories,
} from "../helpers/github-repository-picker";

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1 .2 .4 .5 .9
test.describe("automations: GitHub repository picker", () => {
  test("lists repositories on open, matches a bare name, and keeps the selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedAccessibleRepositories(apiClient, seedData.workspaceId);
    const card = await openRestrictedPrAutomation(
      testPage,
      apiClient,
      seedData,
      "Repository picker round-trip",
    );

    await card.getByTestId("github-repository-add").click();
    const picker = card.getByTestId("github-repository-picker");
    for (const fullName of ACCESSIBLE_REPOSITORIES) {
      await expect(repositoryOptions(picker).filter({ hasText: fullName })).toBeVisible();
    }
    await expectControlHeight(card.getByTestId("github-repository-add"), 28);
    await expectControlHeight(picker.getByTestId("remote-repo-input"), 28);
    await expectControlHeight(picker.getByTestId("remote-repo-refresh"), 28);

    await picker.getByTestId("remote-repo-input").fill("beta");
    await expect(repositoryOptions(picker)).toHaveCount(1);
    await repositoryOptions(picker).filter({ hasText: "mock-user/beta" }).click();
    await expect(card.getByTestId("github-repository-chip")).toHaveText(["mock-user/beta"]);

    await card.getByTestId("github-repository-add").click();
    const input = card.getByTestId("github-repository-picker").getByTestId("remote-repo-input");
    await input.fill("other-org/tool");
    await input.press("Enter");
    await expect(card.getByTestId("github-repository-chip")).toHaveText([
      "mock-user/beta",
      "other-org/tool",
    ]);

    const { saveButton } = new AutomationsPage(testPage, seedData.workspaceId);
    await saveButton.click();
    await expect(saveButton).toBeHidden();

    await testPage.reload();
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible" });
    const reopened = await expandPrTriggerCard(testPage);
    await expect(reopened.getByTestId("github-repository-chip")).toHaveText([
      "mock-user/beta",
      "other-org/tool",
    ]);
  });

  // @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.10 .11
  test("selects an organization and refreshes the repository list", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedAccessibleRepositories(apiClient, seedData.workspaceId);
    await apiClient.mockGitHubAddOrgs([{ login: "kdlbs" }]);
    const card = await openRestrictedPrAutomation(
      testPage,
      apiClient,
      seedData,
      "Organization picker and refresh",
    );

    await card.getByTestId("github-organization-add").click();
    const orgPicker = card.getByTestId("github-organization-picker");
    await expect(organizationOptions(orgPicker)).toHaveText(["mock-user", "kdlbs"]);
    await organizationOptions(orgPicker).filter({ hasText: "kdlbs" }).click();
    await expect(card.getByTestId("github-organization-chip")).toHaveText(["kdlbs"]);

    await card.getByTestId("github-repository-add").click();
    const repoPicker = card.getByTestId("github-repository-picker");
    await expect(repositoryOptions(repoPicker)).toHaveCount(ACCESSIBLE_REPOSITORIES.length);
    await apiClient.mockGitHubAddRepos("mock-user", [
      { full_name: "mock-user/delta", owner: "mock-user", name: "delta", private: false },
    ]);
    await repoPicker.getByTestId("remote-repo-refresh").click();
    await repositoryOptions(repoPicker).filter({ hasText: "mock-user/delta" }).click();
    await expect(card.getByTestId("github-repository-chip")).toHaveText(["mock-user/delta"]);

    const { saveButton } = new AutomationsPage(testPage, seedData.workspaceId);
    await saveButton.click();
    await expect(saveButton).toBeHidden();
    await testPage.reload();
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible" });
    const reopened = await expandPrTriggerCard(testPage);
    await expect(reopened.getByTestId("github-organization-chip")).toHaveText(["kdlbs"]);
    await expect(reopened.getByTestId("github-repository-chip")).toHaveText(["mock-user/delta"]);
  });
});

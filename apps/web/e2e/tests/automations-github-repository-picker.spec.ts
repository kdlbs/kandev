import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";
import {
  ACCESSIBLE_REPOSITORIES,
  expandPrTriggerCard,
  openRestrictedPrAutomation,
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
});

import { test, expect } from "../fixtures/test-base";
import { waitForFiniteAnimations } from "../helpers/animations";
import { expectTouchControl, expectTouchSquareControl } from "../helpers/control-sizing";
import {
  openRestrictedPrAutomation,
  repositoryOptions,
  seedAccessibleRepositories,
} from "../helpers/github-repository-picker";

const PICKER_EDGE_MARGIN = 8;

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.8 .10 .11
test.describe("automations: GitHub repository picker on a phone", () => {
  test("keeps the picker inside the viewport with touch-sized options", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedAccessibleRepositories(apiClient, seedData.workspaceId);
    const card = await openRestrictedPrAutomation(
      testPage,
      apiClient,
      seedData,
      "Mobile repository picker",
    );

    await card.getByTestId("github-repository-add").click();
    const picker = card.getByTestId("github-repository-picker");
    const option = repositoryOptions(picker).filter({ hasText: "mock-user/alpha" });
    await expect(option).toBeVisible();
    await waitForFiniteAnimations(picker);

    const viewport = testPage.viewportSize()!;
    const pickerBox = (await picker.boundingBox())!;
    expect(pickerBox.x).toBeGreaterThanOrEqual(0);
    expect(pickerBox.x + pickerBox.width).toBeLessThanOrEqual(viewport.width);
    await expectTouchControl(option, "repository option");
    await expect(picker.getByTestId("remote-repo-input")).toBeInViewport();
    await expectTouchSquareControl(picker.getByTestId("remote-repo-refresh"));

    await option.tap();
    const chip = card.getByTestId("github-repository-chip");
    await expect(chip).toHaveText(["mock-user/alpha"]);
    const remove = chip.getByRole("button", { name: /remove mock-user\/alpha/i });
    await expectTouchSquareControl(remove);
    await remove.tap();
    await expect(chip).toHaveCount(0);
  });

  test("offers touch-sized organization controls", async ({ testPage, apiClient, seedData }) => {
    await seedAccessibleRepositories(apiClient, seedData.workspaceId);
    await apiClient.mockGitHubAddOrgs([{ login: "kdlbs" }]);
    const card = await openRestrictedPrAutomation(
      testPage,
      apiClient,
      seedData,
      "Mobile organization picker",
    );

    const add = card.getByTestId("github-organization-add");
    await expectTouchControl(add, "add organization");
    await add.tap();
    const picker = card.getByTestId("github-organization-picker");
    const option = picker.getByTestId("github-organization-option").filter({ hasText: "kdlbs" });
    await expect(option).toBeVisible();
    await waitForFiniteAnimations(picker);
    const viewport = testPage.viewportSize()!;
    const pickerBox = (await picker.boundingBox())!;
    expect(pickerBox.x + pickerBox.width).toBeLessThanOrEqual(viewport.width);
    await expectTouchControl(option, "organization option");
    await expectTouchSquareControl(picker.getByTestId("github-organization-refresh"));

    await option.tap();
    const chip = card.getByTestId("github-organization-chip");
    await expect(chip).toHaveText(["kdlbs"]);
    await expectTouchSquareControl(chip.getByRole("button", { name: /remove kdlbs/i }));

    // The chip pushes the trigger right; the reopened picker must keep a margin from the edge.
    await add.tap();
    await expect(picker.getByTestId("github-organization-option")).toHaveCount(2);
    await waitForFiniteAnimations(picker);
    const shifted = (await picker.boundingBox())!;
    expect(shifted.x).toBeGreaterThanOrEqual(PICKER_EDGE_MARGIN - 1);
    expect(shifted.x + shifted.width).toBeLessThanOrEqual(viewport.width - PICKER_EDGE_MARGIN + 1);
  });

  test("keeps both pickers touch-sized after resizing to a tablet width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 820, height: 1180 });
    await seedAccessibleRepositories(apiClient, seedData.workspaceId);
    await apiClient.mockGitHubAddOrgs([{ login: "kdlbs" }]);
    const card = await openRestrictedPrAutomation(
      testPage,
      apiClient,
      seedData,
      "Tablet repository picker controls",
    );

    const addRepository = card.getByTestId("github-repository-add");
    await expectTouchControl(addRepository);
    await addRepository.tap();
    const repositoryPicker = card.getByTestId("github-repository-picker");
    const repository = repositoryOptions(repositoryPicker).filter({ hasText: "mock-user/alpha" });
    await expect(repository).toBeVisible();
    await waitForFiniteAnimations(repositoryPicker);
    await expectTouchControl(repositoryPicker.getByTestId("remote-repo-input"));
    await expectTouchSquareControl(repositoryPicker.getByTestId("remote-repo-refresh"));
    await expectTouchControl(repository);
    await repository.tap();
    const removeRepository = card
      .getByTestId("github-repository-chip")
      .getByRole("button", { name: /remove mock-user\/alpha/i });
    await expectTouchSquareControl(removeRepository);
    await removeRepository.tap();

    const addOrganization = card.getByTestId("github-organization-add");
    await expectTouchControl(addOrganization);
    await addOrganization.tap();
    const organizationPicker = card.getByTestId("github-organization-picker");
    const organization = organizationPicker
      .getByTestId("github-organization-option")
      .filter({ hasText: "kdlbs" });
    await expect(organization).toBeVisible();
    await waitForFiniteAnimations(organizationPicker);
    await expectTouchControl(organizationPicker.getByTestId("github-organization-input"));
    await expectTouchSquareControl(organizationPicker.getByTestId("github-organization-refresh"));
    await expectTouchControl(organization);
    await organization.tap();
    await expectTouchSquareControl(
      card.getByTestId("github-organization-chip").getByRole("button", { name: /remove kdlbs/i }),
    );
  });
});

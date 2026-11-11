import { test, expect } from "../fixtures/test-base";
import { waitForFiniteAnimations } from "../helpers/animations";
import { expectTouchControl, expectTouchSquareControl } from "../helpers/control-sizing";
import {
  openRestrictedPrAutomation,
  repositoryOptions,
  seedAccessibleRepositories,
} from "../helpers/github-repository-picker";

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.8
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

    await option.tap();
    const chip = card.getByTestId("github-repository-chip");
    await expect(chip).toHaveText(["mock-user/alpha"]);
    const remove = chip.getByRole("button", { name: /remove mock-user\/alpha/i });
    await expectTouchSquareControl(remove);
    await remove.tap();
    await expect(chip).toHaveCount(0);
  });
});

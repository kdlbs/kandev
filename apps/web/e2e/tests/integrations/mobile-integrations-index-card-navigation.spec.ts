import { expect, test } from "../../fixtures/test-base";

test.describe("integrations index card navigation on mobile", () => {
  test("tapping a card body opens the workspace-scoped integration settings page", async ({
    testPage,
    seedData,
  }) => {
    const integrationsPath = `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations`;
    await testPage.goto(integrationsPath);

    const card = testPage.getByTestId("integration-card-github");
    await expect(card).toBeVisible();

    const cardBox = await card.boundingBox();
    const descriptionBox = await card.locator("p").boundingBox();
    expect(cardBox).not.toBeNull();
    expect(descriptionBox).not.toBeNull();
    await card.tap({
      position: {
        x: descriptionBox!.x - cardBox!.x + descriptionBox!.width / 2,
        y: descriptionBox!.y - cardBox!.y + descriptionBox!.height / 2,
      },
    });

    await expect(testPage).toHaveURL(`${integrationsPath}/github`);
    await expect(testPage.getByTestId("github-integration-heading")).toBeVisible();
  });
});

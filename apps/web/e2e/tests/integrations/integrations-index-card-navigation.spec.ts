import { expect, test } from "../../fixtures/test-base";

test.describe("integrations index card navigation", () => {
  test("clicking a card body opens the workspace-scoped integration settings page", async ({
    testPage,
    seedData,
  }) => {
    const integrationsPath = `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations`;
    await testPage.goto(integrationsPath);

    const card = testPage.getByTestId("integration-card-github");
    await expect(card).toBeVisible();

    const cardBox = await card.boundingBox();
    expect(cardBox).not.toBeNull();
    await card.click({ position: { x: 16, y: cardBox!.height - 12 } });

    await expect(testPage).toHaveURL(`${integrationsPath}/github`);
    await expect(testPage.getByTestId("github-integration-heading")).toBeVisible();
  });
});

import { expect } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

async function createActiveTask(apiClient: ApiClient, seedData: SeedData) {
  return apiClient.createTask(seedData.workspaceId, "Mobile saved-layout confirmation task", {
    description: "Mobile saved-layout confirmation",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

function minimalLayout() {
  return {
    columns: [
      {
        id: "center",
        groups: [
          {
            id: "group-center",
            panels: [
              {
                id: "chat",
                component: "chat",
                title: "Agent",
              },
            ],
            activePanel: "chat",
          },
        ],
      },
    ],
  };
}

test.describe("Mobile saved-layout confirmation", () => {
  test("morphs the saved-layout menu row into touch-sized actions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await createActiveTask(apiClient, seedData);
    await apiClient.saveUserSettings({
      saved_layouts: [
        {
          id: "mobile-layout-default",
          name: "Mobile default",
          is_default: true,
          layout: minimalLayout(),
          created_at: new Date().toISOString(),
        },
        {
          id: "mobile-layout-to-delete",
          name: "Mobile layout to delete",
          is_default: false,
          layout: minimalLayout(),
          created_at: new Date().toISOString(),
        },
      ],
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const presetTrigger = testPage.getByTestId("layout-preset-trigger");
    await expect(presetTrigger).toBeVisible({ timeout: 30_000 });
    await expect(presetTrigger).toHaveAccessibleName("Saved Layouts");
    await presetTrigger.tap();
    await expect(testPage.getByTestId("layout-reset-item")).toHaveCount(0);
    await expect(testPage.getByTestId("layout-preset-item")).toHaveCount(0);
    await expect(testPage.getByText("Save current layout...", { exact: true })).toHaveCount(0);

    const deleteAction = testPage.locator(
      '[data-testid="layout-saved-delete"][data-layout-id="mobile-layout-to-delete"]',
    );
    await expect(deleteAction).toBeVisible({ timeout: 15_000 });
    await deleteAction.tap();

    const inline = testPage.getByTestId("layout-saved-delete-inline-confirmation");
    await expect(inline).toBeVisible();
    await expect(testPage.getByTestId("layout-saved-delete-confirm-popover")).toHaveCount(0);
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    for (const action of await inline.getByRole("button").all()) {
      const box = await action.boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }

    await inline.getByRole("button", { name: "Cancel" }).tap();
    expect((await apiClient.getUserSettings()).settings.saved_layouts).toHaveLength(2);

    if ((await presetTrigger.getAttribute("aria-expanded")) !== "true") {
      await presetTrigger.tap();
    }
    await expect(deleteAction).toBeVisible({ timeout: 15_000 });
    await deleteAction.tap();
    await testPage
      .getByTestId("layout-saved-delete-inline-confirmation")
      .getByTestId("layout-saved-delete-confirm")
      .tap();
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.saved_layouts)
      .toHaveLength(1);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile saved-layout confirmation");
  });
});

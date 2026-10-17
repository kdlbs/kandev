import { expect, test } from "../../fixtures/office-fixture";
import {
  assertAggregateRunOpensInSourceWorkspace,
  createAggregateNavigationTarget,
} from "../../helpers/workspace-aggregate-navigation";

test("mobile aggregate activity opens a run in its source workspace", async ({
  testPage,
  apiClient,
  officeApi,
  officeSeed,
  seedData,
}) => {
  const target = await createAggregateNavigationTarget(
    apiClient,
    officeApi,
    seedData.agentProfileId,
    "Mobile aggregate target workspace",
  );

  try {
    await testPage.goto(`/office/tasks?workspaceId=${encodeURIComponent(officeSeed.workspaceId)}`);
    await testPage.getByTestId("app-nav-trigger").tap();
    const navSheet = testPage.getByTestId("app-nav-sheet");
    await navSheet.getByRole("link", { name: "Overview", exact: true }).tap();
    await expect(testPage).toHaveURL(/\/office\/overview(?:\?.*)?$/);

    await assertAggregateRunOpensInSourceWorkspace(testPage, officeSeed.workspaceId, target, true);
  } finally {
    await testPage.goto(`/office?workspaceId=${encodeURIComponent(officeSeed.workspaceId)}`);
    await testPage.locator(`main[data-office-route="/office"]`).waitFor();
    await officeApi.deleteWorkspace(target.workspaceId, target.workspaceName);
  }
});

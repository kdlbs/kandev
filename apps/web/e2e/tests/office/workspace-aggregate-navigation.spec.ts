import { test } from "../../fixtures/office-fixture";
import {
  assertAggregateRunOpensInSourceWorkspace,
  createAggregateNavigationTarget,
} from "../../helpers/workspace-aggregate-navigation";

test("aggregate activity opens a run in its source workspace", async ({
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
    "Aggregate target workspace",
  );

  try {
    await assertAggregateRunOpensInSourceWorkspace(testPage, officeSeed.workspaceId, target);
  } finally {
    await testPage.goto(`/office?workspaceId=${encodeURIComponent(officeSeed.workspaceId)}`);
    await testPage.locator(`main[data-office-route="/office"]`).waitFor();
    await officeApi.deleteWorkspace(target.workspaceId, target.workspaceName);
  }
});

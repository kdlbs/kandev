import { test } from "../../fixtures/test-base";
import { expectStableIntegrationCardLayout } from "./integrations-index-layout-helpers";

test.describe("integrations settings index layout on mobile", () => {
  test("renders equal-height integration cards on a workspace-scoped route", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations`);

    await expectStableIntegrationCardLayout(testPage);
  });
});

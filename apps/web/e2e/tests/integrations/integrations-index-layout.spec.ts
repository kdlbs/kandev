import { test } from "../../fixtures/test-base";
import { expectStableIntegrationCardLayout } from "./integrations-index-layout-helpers";

test.describe("integrations settings index layout", () => {
  test("renders equal-height integration cards on desktop", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1024, height: 900 });
    await testPage.goto("/settings/integrations");

    await expectStableIntegrationCardLayout(testPage);
  });
});

import { test } from "../../fixtures/test-base";
import { expectStableIntegrationCardLayout } from "./integrations-index-layout-helpers";

test.describe("integrations settings index layout on mobile", () => {
  test("renders equal-height integration cards", async ({ testPage }) => {
    await testPage.goto("/settings/integrations");

    await expectStableIntegrationCardLayout(testPage);
  });
});

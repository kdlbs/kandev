import { test } from "../../fixtures/test-base";
import { runWorkspaceStreamPromotion } from "./workspace-stream-promotion-helpers";

test.describe("Workspace stream promotion in phone Changes", () => {
  test("delivers file membership and settled detail after prepared-session startup", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(240_000);
    await runWorkspaceStreamPromotion({
      page: testPage,
      apiClient,
      seedData,
      prCapture,
      testInfo,
      mobile: true,
    });
  });
});

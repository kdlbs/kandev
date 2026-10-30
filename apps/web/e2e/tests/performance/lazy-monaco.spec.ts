import { test, expect, resetSeedRepositoryCheckout } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { verifyLazyMonaco } from "../../helpers/lazy-monaco";

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-004.2
test("loads Monaco when the file editor first opens", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}, info) => {
  test.setTimeout(90_000);
  resetSeedRepositoryCheckout(seedData, backend.tmpDir);
  await verifyLazyMonaco(testPage, apiClient, seedData, backend, { info });
  if (prCapture.capturing) {
    await new SessionPage(testPage).clickMaximize("lazy-editor.ts");
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "lazyEditorReady",
    );
    await expect(testPage.locator(".monaco-editor:visible .view-lines")).toContainText(
      "lazy input works",
    );
  }
  await prCapture.screenshot("desktop-editor");
});

test.afterEach(async ({ seedData, backend }) => {
  resetSeedRepositoryCheckout(seedData, backend.tmpDir);
});

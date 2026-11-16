import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  checkBulkPreviewRecovery,
  expectBulkManager,
  installBulkRuntimeFixture,
} from "./agent-runtime-bulk-update-helpers";

// @covers AC-AGENTS-RUNTIME-MANAGER-002.4, AC-AGENTS-RUNTIME-MANAGER-003.4
test("failed checks explain the cause and retry without installing updates", async ({
  testPage,
  prCapture,
}) => {
  await checkBulkPreviewRecovery(testPage, false, () =>
    prCapture.screenshot("runtime-update-manager-desktop", {
      caption: "Runtime manager with a preview failure reason and one ready update",
    }),
  );
});

// @covers AC-AGENTS-RUNTIME-MANAGER-003.1, AC-AGENTS-RUNTIME-MANAGER-003.2
test("bulk manager reviews exact targets and approves only the selection", async ({ testPage }) => {
  const runtime = await installBulkRuntimeFixture(testPage);
  await testPage.goto("/settings/agents");
  await expect(testPage.getByTestId("runtime-updates-button")).toBeVisible();
  await testPage.getByTestId("runtime-updates-button").click();
  await expectBulkManager(testPage, false);
  const approve = testPage.getByTestId("runtime-bulk-approve");
  await expect(approve).toContainText("Update all (2)");

  await testPage.getByTestId("runtime-bulk-select-all").click();
  await expect(approve).toBeDisabled();
  await testPage.getByTestId("runtime-bulk-select-codex-app-server").click();
  await expect(approve).toContainText("Update selected (1)");
  await expect(approve).toBeEnabled();

  await approve.click();
  await expect(testPage.getByTestId("runtime-batch-row-codex-app-server")).toBeVisible();
  await expect(testPage.getByTestId("runtime-batch-row-claude-acp")).toHaveCount(0);
  await expect.poll(() => runtime.codexPosts()).toBe(1);
  expect(runtime.claudePosts()).toBe(0);
});

// @covers AC-AGENTS-RUNTIME-MANAGER-003.3, AC-AGENTS-RUNTIME-MANAGER-003.4
test("partial admission failure retries only the failed target once", async ({ testPage }) => {
  const runtime = await installBulkRuntimeFixture(testPage);
  await testPage.goto("/settings/agents");
  await testPage.getByTestId("runtime-updates-button").click();
  await expectBulkManager(testPage, false);

  await testPage.getByTestId("runtime-bulk-approve").click();
  await expect(testPage.getByTestId("runtime-batch-row-claude-acp")).toBeVisible();
  await expect(testPage.getByTestId("runtime-batch-row-codex-app-server")).toBeVisible();
  await expect.poll(() => runtime.claudePosts()).toBe(1);
  await expect.poll(() => runtime.codexPosts()).toBe(1);

  await runtime.finishClaude();
  await expect(testPage.getByTestId("runtime-batch-retry-failed")).toBeVisible();
  await expect(testPage.getByTestId("runtime-batch-summary")).toContainText("1 failed");

  runtime.setCodexSucceeds(true);
  await testPage.getByTestId("runtime-batch-retry-failed").click();
  await expect(testPage.getByTestId("runtime-bulk-approve")).toContainText("Update all (1)");
  await testPage.getByTestId("runtime-bulk-approve").click();
  await expect.poll(() => runtime.codexPosts()).toBe(2);
  expect(runtime.claudePosts()).toBe(1);

  await runtime.finishCodex();
  await expect(testPage.getByTestId("runtime-batch-summary")).toContainText("2 updated");
  await testPage.getByTestId("runtime-batch-review-again").click();
  await expect(testPage.getByTestId("runtime-bulk-approve")).toBeVisible();
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
    false,
  );
});

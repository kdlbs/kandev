import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import {
  checkBulkPreviewRecovery,
  expectBulkManager,
  installBulkRuntimeFixture,
} from "./agent-runtime-bulk-update-helpers";

// @covers AC-AGENTS-RUNTIME-MANAGER-001.6, AC-AGENTS-RUNTIME-MANAGER-002.4
test("phone failed checks explain the cause and retry without installing updates", async ({
  testPage,
}) => {
  await checkBulkPreviewRecovery(testPage, true);
});

// @covers AC-AGENTS-RUNTIME-MANAGER-001.6, AC-AGENTS-RUNTIME-MANAGER-003.2
test("phone bulk review keeps one scrolling drawer and touch-sized approval", async ({
  testPage,
}) => {
  const runtime = await installBulkRuntimeFixture(testPage);
  await testPage.goto("/settings/agents");
  const entry = testPage.getByTestId("runtime-updates-button");
  expect((await entry.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await entry.tap();
  const surface = await expectBulkManager(testPage, true);
  await expect(surface).toBeInViewport();
  const approve = testPage.getByTestId("runtime-bulk-approve");
  expect((await approve.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await expect(approve).toContainText("Update all (2)");

  await testPage.getByTestId("runtime-bulk-select-all").tap();
  await expect(approve).toBeDisabled();
  await testPage.getByTestId("runtime-bulk-select-claude-acp").tap();
  await expect(approve).toContainText("Update selected (1)");
  await approve.tap();
  await expect(testPage.getByTestId("runtime-batch-row-claude-acp")).toBeVisible();
  await expect.poll(() => runtime.claudePosts()).toBe(1);
  expect(runtime.codexPosts()).toBe(0);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
    false,
  );
});

// @covers AC-AGENTS-RUNTIME-MANAGER-003.4, AC-AGENTS-RUNTIME-MANAGER-003.5
test("phone failed-only retry preserves successful rows after dismissal", async ({ testPage }) => {
  const runtime = await installBulkRuntimeFixture(testPage);
  await testPage.goto("/settings/agents");
  await testPage.getByTestId("runtime-updates-button").tap();
  await expectBulkManager(testPage, true);
  await testPage.getByTestId("runtime-bulk-approve").tap();
  await expect(testPage.getByTestId("runtime-batch-row-codex-app-server")).toBeVisible();

  await testPage.getByTestId("runtime-manager-close").tap();
  await expect(testPage.getByTestId("runtime-manager-drawer")).toHaveCount(0);
  await testPage.getByTestId("runtime-updates-button").tap();
  await expect(testPage.getByTestId("runtime-batch-row-codex-app-server")).toBeVisible();

  await runtime.finishClaude();
  await expect(testPage.getByTestId("runtime-batch-retry-failed")).toBeVisible();

  runtime.setCodexSucceeds(true);
  await testPage.getByTestId("runtime-batch-retry-failed").tap();
  await expect(testPage.getByTestId("runtime-bulk-approve")).toContainText("Update all (1)");
  await testPage.getByTestId("runtime-bulk-approve").tap();
  await expect.poll(() => runtime.codexPosts()).toBe(2);
  expect(runtime.claudePosts()).toBe(1);
  await runtime.finishCodex();
  await expect(testPage.getByTestId("runtime-batch-summary")).toContainText("2 updated");
});

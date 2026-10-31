import { test, expect } from "../../fixtures/test-base";
import {
  activateRetention,
  analyzeRetention,
  expectTouchTarget,
  failNextRetentionStatusRead,
  failRetentionAnalysis,
  readRetentionMessage,
  recoverRetentionStatusPolling,
  reloadRetentionWithFakeClock,
  resetRetention,
  seedRetentionAnalysis,
  RETENTION_ROUTE,
  runRetention,
  seedRetentionTask,
  TOOL_COMMAND,
} from "../../helpers/tool-payload-retention";

test.describe("Tool payload retention on phones", () => {
  test("recovers a status polling error without manual refresh on a phone", async ({
    testPage: page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await resetRetention(page);
    await seedRetentionAnalysis(page);
    await page.goto(RETENTION_ROUTE);
    await reloadRetentionWithFakeClock(page);
    const removeStatusRoute = await failNextRetentionStatusRead(page);
    try {
      await recoverRetentionStatusPolling(page);
    } finally {
      await removeStatusRoute();
    }
  });

  test("preserves an action failure after status recovery on a phone", async ({
    testPage: page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await resetRetention(page);
    await seedRetentionAnalysis(page);
    await page.goto(RETENTION_ROUTE);
    await reloadRetentionWithFakeClock(page);
    const removeStatusRoute = await failNextRetentionStatusRead(page);
    const removeActionRoute = await failRetentionAnalysis(page, true);
    try {
      await recoverRetentionStatusPolling(page, true);
    } finally {
      await removeActionRoute();
      await removeStatusRoute();
    }
  });

  for (const width of [320, 390]) {
    test(`completes cleanup by touch at ${width}px without clipping`, async ({
      testPage: page,
      backend,
      apiClient,
      seedData,
    }) => {
      await page.setViewportSize({ width, height: 844 });
      await resetRetention(page);
      const old = await seedRetentionTask(backend.tmpDir, apiClient, seedData, false);
      const recent = await seedRetentionTask(backend.tmpDir, apiClient, seedData, true);
      const protectedOriginal = readRetentionMessage(backend.tmpDir, recent.messageId);
      try {
        await page.goto(RETENTION_ROUTE);
        await expect(page.getByTestId("tool-payload-enabled")).not.toBeChecked();
        for (const id of ["tool-payload-age", "tool-payload-unit", "tool-payload-analyze"]) {
          await expectTouchTarget(page.getByTestId(id));
        }
        await expectTouchTarget(page.locator('label[for="tool-payload-enabled"]'));
        await expect(
          page.getByText("Checks every 24 hours while Kandev is running."),
        ).toBeVisible();
        const ageBox = await page.getByTestId("tool-payload-age").boundingBox();
        const unitBox = await page.getByTestId("tool-payload-unit").boundingBox();
        expect(Math.abs(ageBox!.y - unitBox!.y)).toBeLessThan(2);
        await analyzeRetention(page, true);
        await page.getByTestId("tool-payload-retention-card").screenshot({
          path: `/tmp/compaction-mobile-${width}.png`,
        });
        const details = page.getByTestId("tool-payload-estimate").locator("details");
        await expect(details).not.toHaveAttribute("open");
        await details.locator("summary").tap();
        await expect(details).toHaveAttribute("open");
        await details.locator("summary").tap();
        await page.getByTestId("tool-payload-age").fill("26");
        await page.getByTestId("tool-payload-unit").tap();
        await page.getByRole("option", { name: "Weeks", exact: true }).tap();
        await expect(page.getByTestId("tool-payload-estimate")).toContainText(
          "This estimate is stale",
        );
        await analyzeRetention(page, true);
        await activateRetention(page, "skip", true);
        await expect(page.getByTestId("tool-payload-age")).toHaveValue("26");
        await expectTouchTarget(page.getByTestId("tool-payload-run"));
        await runRetention(page, true);
        const reduced = readRetentionMessage(backend.tmpDir, old.messageId);
        expect(reduced.metadata.payload_retention.version).toBe(1);
        expect(reduced.metadata.normalized.shell_exec.output).not.toHaveProperty("stdout");
        expect(reduced.metadata.normalized.shell_exec.command).toBe(TOOL_COMMAND);
        expect(readRetentionMessage(backend.tmpDir, recent.messageId)).toEqual(protectedOriginal);
        await expect
          .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
          .toBe(true);
        await page.reload();
        await expect(page.getByTestId("tool-payload-enabled")).toBeChecked();
        await expect(page.getByTestId("tool-payload-last-run")).toContainText("Completed");
        await expect
          .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
          .toBe(true);
        await page.goto(`/t/${old.taskId}`);
        await expect(page.getByTestId("tool-execute-command")).toHaveText(TOOL_COMMAND);
        await expect(page.getByText(/^Tool details removed on /)).toBeVisible();
        await page.reload();
        await expect(page.getByText(/^Tool details removed on /)).toBeVisible();
      } finally {
        await resetRetention(page);
        await apiClient.deleteTask(old.taskId);
        await apiClient.deleteTask(recent.taskId);
      }
    });
  }
});

test.describe("History compaction categories on phones", () => {
  test("keeps one control set and category details without horizontal overflow", async ({
    testPage: page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const status = {
      supported: true,
      policy: { enabled: true, age: { value: 3, unit: "months" }, revision: 4 },
      preparation: { state: "ready", choice: "skip" },
      last_analysis: {
        id: "mixed-analysis",
        kind: "analysis",
        state: "succeeded",
        scanned: 4,
        eligible_tasks: 2,
        eligible_messages: 3,
        removed_messages: 0,
        payload_bytes: 2048,
        history_tasks: 2,
        history_snapshots: 5,
        history_sql_bytes: 1024,
        history_git_bytes: 4096,
        expired_change_sets: 1,
        deleted_archive_refs: 2,
        archive_failures: 0,
        skipped: {},
        cutoff: "2026-01-01T00:00:00.000Z",
        started_at: "2026-01-01T00:00:00.000Z",
        finished_at: "2026-01-01T00:01:00.000Z",
        age: { value: 3, unit: "months" },
      },
    };
    await page.route("**/api/v1/system/database/tool-payload-retention", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(status) }),
    );
    await page.goto(RETENTION_ROUTE);
    const card = page.getByTestId("tool-payload-retention-card");
    await expect(card).toContainText("History compaction");
    await expect(card).toContainText("Historical snapshots: 5 across 2 tasks");
    await expect(page.getByTestId("tool-payload-enabled")).toHaveCount(1);
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });
});

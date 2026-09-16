import { test, expect } from "../../fixtures/test-base";
import {
  activateRetention,
  analyzeRetention,
  expectTouchTarget,
  readRetentionMessage,
  resetRetention,
  RETENTION_ROUTE,
  runRetention,
  seedRetentionTask,
  TOOL_COMMAND,
} from "../../helpers/tool-payload-retention";

test.describe("Tool payload retention on phones", () => {
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

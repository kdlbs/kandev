import { readFile } from "node:fs/promises";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

const INITIAL = "## Plan\n\nOriginal plan.";
const EXPECTED = "## Plan\n\nOriginal plan. Café 🌍\n\nNext line 雪";

export async function openDownloadPlan(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Plan download",
    seed.agentProfileId,
    {
      description: planScript(INITIAL),
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect.poll(() => api.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
  await session.waitForChatIdle({ timeout: 45_000 });
  await session.composerReady();
  if (mobile || !(await session.planPanel.isVisible())) await session.togglePlanMode();
  if (mobile)
    await page.getByRole("navigation").getByRole("button", { name: "Plan", exact: true }).click();
  const panel = page.getByTestId("plan-panel");
  await expect(panel).toContainText("Original plan.");
  const editor = panel.locator(".ProseMirror");
  await editor.click();
  await page.keyboard.press(process.platform === "darwin" ? "Meta+End" : "Control+End");
  await page.keyboard.insertText(" Café 🌍");
  await page.keyboard.press("Enter");
  await page.keyboard.insertText("Next line 雪");
  await expect(editor).toContainText("Next line 雪");
  return panel;
}

export async function checkDownload(page: Page, mobile: boolean) {
  const panel = page.getByTestId("plan-panel");
  const button = panel.getByRole("button", { name: "Download", exact: true });
  await expect(button).toBeEnabled();
  const downloadPromise = page.waitForEvent("download");
  if (mobile) await button.tap();
  else {
    await button.focus();
    await page.keyboard.press("Enter");
  }
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("plan.md");
  const path = await download.path();
  expect(path).not.toBeNull();
  expect(await readFile(path!, "utf8")).toBe(EXPECTED);
  await expect(panel.locator(".ProseMirror")).toContainText("Café 🌍");
  await expect(panel.locator(".ProseMirror")).toContainText("Next line 雪");
}

export async function checkGeometry(page: Page, touchSize: boolean) {
  const panel = page.getByTestId("plan-panel");
  const button = panel.getByRole("button", { name: "Download", exact: true });
  const box = await button.boundingBox();
  expect(box).not.toBeNull();
  if (touchSize) {
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);
  } else expect(box!.height).toBeCloseTo(28, 0);
  const geometry = await button.evaluate((element) => {
    const header = element.closest(".border-b")!;
    const bounds = header.getBoundingClientRect();
    const controls = Array.from(header.querySelectorAll("button")).map((control) => {
      const rect = control.getBoundingClientRect();
      return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom };
    });
    return {
      bounds: { left: bounds.left, right: bounds.right, top: bounds.top, bottom: bounds.bottom },
      controls,
    };
  });
  for (const control of geometry.controls) {
    expect(control.left).toBeGreaterThanOrEqual(geometry.bounds.left);
    expect(control.right).toBeLessThanOrEqual(geometry.bounds.right);
    expect(control.top).toBeGreaterThanOrEqual(geometry.bounds.top);
    expect(control.bottom).toBeLessThanOrEqual(geometry.bounds.bottom);
  }
  await expect(panel.getByTestId("plan-toolbar-implement-button")).toBeVisible();
  await expect(panel.getByTestId("plan-rewind-button")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
    ),
  ).toBe(true);
}

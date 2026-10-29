import { readFile } from "node:fs/promises";
import { expect, type Page } from "@playwright/test";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

export async function openExport(page: Page, workspaceId: string, single: boolean, touch: boolean) {
  const settings = new WorkflowSettingsPage(page);
  await settings.goto(workspaceId);
  const button = single
    ? (await settings.findWorkflowCard("Café 雪 workflow")).getByRole("button", {
        name: "Export",
        exact: true,
      })
    : page.getByRole("button", { name: "Export All", exact: true });
  if (touch) await button.tap();
  else await button.click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await expect(dialog.locator("textarea")).toHaveValue(/Café 雪 workflow/);
  await dialog.evaluate(async (element) => {
    await Promise.all(
      element.getAnimations().map((animation) => animation.finished.catch(() => {})),
    );
  });
  return dialog;
}

export async function checkGeometry(page: Page, touchSize: boolean) {
  const dialog = page.getByRole("dialog");
  const footer = dialog.locator('[data-slot="dialog-footer"]');
  const bounds = await footer.boundingBox();
  const dialogBounds = await dialog.boundingBox();
  expect(bounds).not.toBeNull();
  expect(dialogBounds).not.toBeNull();
  const viewport = page.viewportSize()!;
  expect(dialogBounds!.x).toBeGreaterThanOrEqual(0);
  expect(dialogBounds!.y).toBeGreaterThanOrEqual(0);
  expect(dialogBounds!.x + dialogBounds!.width).toBeLessThanOrEqual(viewport.width);
  expect(dialogBounds!.y + dialogBounds!.height).toBeLessThanOrEqual(viewport.height);
  const textarea = dialog.locator("textarea");
  expect(
    await textarea.evaluate((element) => {
      const canScroll = element.scrollHeight > element.clientHeight;
      element.scrollTop = 20;
      const scrolled = element.scrollTop > 0;
      element.scrollTop = 0;
      return canScroll && scrolled;
    }),
  ).toBe(true);
  for (const name of ["Close", "Download", "Copy"]) {
    const button = footer.getByRole("button", { name, exact: true });
    const box = await button.boundingBox();
    expect(box).not.toBeNull();
    if (touchSize) {
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
    } else expect(box!.height).toBeCloseTo(28, 0);
    expect(box!.x).toBeGreaterThanOrEqual(bounds!.x - 1);
    expect(box!.x + box!.width).toBeLessThanOrEqual(bounds!.x + bounds!.width + 1);
    expect(box!.y).toBeGreaterThanOrEqual(bounds!.y - 1);
    expect(box!.y + box!.height).toBeLessThanOrEqual(bounds!.y + bounds!.height + 1);
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.y).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
    expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.height);
    expect(
      await button.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        const hit = document.elementFromPoint(
          rect.left + rect.width / 2,
          rect.top + rect.height / 2,
        );
        return hit === element || element.contains(hit);
      }),
    ).toBe(true);
  }
  expect(bounds!.x).toBeGreaterThanOrEqual(dialogBounds!.x - 1);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(dialogBounds!.x + dialogBounds!.width + 1);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

export async function checkDownload(page: Page, touch: boolean) {
  const dialog = page.getByRole("dialog");
  const current = await dialog.locator("textarea").inputValue();
  expect(current).toContain("Café 雪 workflow");
  expect(current).toContain("\n");
  const button = dialog.getByRole("button", { name: "Download", exact: true });
  if (!touch) {
    for (let i = 0; i < 8 && !(await button.evaluate((el) => el === document.activeElement)); i++) {
      await page.keyboard.press("Tab");
    }
    await expect(button).toBeFocused();
  }
  const event = page.waitForEvent("download");
  if (touch) await button.tap();
  else await page.keyboard.press("Enter");
  const download = await event;
  expect(download.suggestedFilename()).toBe("workflows.yaml");
  const file = await download.path();
  expect(file).not.toBeNull();
  expect(await readFile(file!)).toEqual(Buffer.from(current, "utf8"));
  await expect(dialog.locator("textarea")).toHaveValue(current);
  await expect(dialog.getByRole("button", { name: "Copy", exact: true })).toBeVisible();
}

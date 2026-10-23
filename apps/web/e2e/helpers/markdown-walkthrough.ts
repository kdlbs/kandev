import { expect, type Locator, type Page } from "@playwright/test";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { DatabaseSync } from "./node-sqlite";

export function seedMarkdownWalkthrough(
  tmpDir: string,
  taskId: string,
  file: string,
  content: string,
  repo?: string,
) {
  const lines = content.split("\n");
  const first = lines.findIndex((line) => line.startsWith("| Area")) + 1;
  const distant = lines.findIndex((line) => line.startsWith("Long") && line.includes("45 ")) + 1;
  expect(first).toBeGreaterThan(0);
  expect(distant).toBeGreaterThan(first);
  const ranges = [
    { line: first, line_end: first + 2 },
    { line: distant, line_end: distant },
    { line: distant, line_end: distant + 4 },
  ];
  const steps = ranges.map((range, index) => ({
    ...range,
    file,
    ...(repo ? { repo } : {}),
    title: `Section ${index + 1}`,
    text: `Inspect Markdown section ${index + 1}.`,
  }));
  // Seed the persisted producer shape. The UI loads it through task.walkthrough.get.
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    db.exec("PRAGMA busy_timeout = 10000");
    const now = new Date().toISOString();
    db.prepare(
      "INSERT INTO task_walkthroughs (id, task_id, title, steps, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
    ).run(randomUUID(), taskId, "Markdown sections", JSON.stringify(steps), "agent", now, now);
  } finally {
    db.close();
  }
  return ranges;
}

export async function openMarkdownWalkthrough(page: Page) {
  await page.reload();
  const launcher = page.getByTestId("walkthrough-launcher");
  await expect(launcher).toBeVisible();
  await launcher.click();
  const card = page.getByTestId("walkthrough-floating");
  await expect(card).toBeVisible();
  if ((page.viewportSize()?.width ?? 0) >= 768) {
    const handle = await card.getByTestId("walkthrough-drag-handle").boundingBox();
    if (!handle) throw new Error("Missing walkthrough drag handle");
    await page.mouse.move(handle.x + 60, handle.y + 12);
    await page.mouse.down();
    await page.mouse.move(76, 152, { steps: 6 });
    await page.mouse.up();
  }
  return card;
}

export async function expectMarkdownWalkthroughRange(
  surface: Locator,
  range: { line: number; line_end: number },
  target: Locator,
) {
  const indicator = surface.getByTestId("walkthrough-editor-range");
  await expect(indicator).toHaveCount(1);
  await expect(indicator).toBeVisible();
  await expect(indicator).toHaveAttribute("data-line-range", `${range.line}-${range.line_end}`);
  await expect(indicator).toHaveCSS("pointer-events", "none");
  await expect(indicator).toHaveCSS("border-left-width", "2px");
  await expect
    .poll(async () => {
      const [outline, block] = await Promise.all([indicator.boundingBox(), target.boundingBox()]);
      if (!outline || !block) return false;
      return (
        outline.y >= block.y - 16 &&
        outline.y <= block.y + 4 &&
        outline.y + outline.height >= block.y + block.height - 4
      );
    })
    .toBe(true);
  const colors = await indicator.evaluate((element) => {
    const style = getComputedStyle(element);
    return { border: style.borderLeftColor, fill: style.backgroundColor };
  });
  expect(colors.border).not.toBe("rgba(0, 0, 0, 0)");
  expect(colors.fill).not.toBe("rgba(0, 0, 0, 0)");
  return indicator;
}

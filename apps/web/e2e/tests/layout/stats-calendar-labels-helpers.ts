import { expect, type Locator, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import { waitForHttp } from "../../helpers/causal-waits";
import type { CompletedTaskActivityDTO, DailyActivityDTO } from "../../../lib/types/http";

const completedActivity: CompletedTaskActivityDTO[] = [
  { date: "2025-12-31", completed_tasks: 2 },
  { date: "2026-01-01", completed_tasks: 3 },
  { date: "2026-01-05", completed_tasks: 4 },
];
const dailyActivity: DailyActivityDTO[] = [
  { date: "2026-01-01", turn_count: 2, message_count: 3, task_count: 1 },
  ...["2026-01-02", "2026-01-03", "2026-01-04", "2026-01-05"].map((date) => ({
    date,
    turn_count: 0,
    message_count: 0,
    task_count: 0,
  })),
];

// Literal oracles describe the API calendar keys, independently of Date formatting.
const buckets = [
  { name: "Day", labels: ["Dec 31", "Jan 1", "Jan 5"], counts: [2, 3, 4], heights: [50, 75, 100] },
  { name: "Week", labels: ["Dec 29", "Jan 5"], counts: [5, 4], heights: [100, 80] },
  { name: "Month", labels: ["Dec 25", "Jan 26"], counts: [2, 7], heights: [29, 100] },
];

export async function openCalendarStats(
  page: Page,
  backend: BackendContext,
  workspaceId: string,
  timezoneId: string,
): Promise<Locator> {
  await page.addInitScript((port) => {
    localStorage.setItem("kandev.onboarding.completed", "true");
    window.__KANDEV_API_PORT = port;
  }, String(backend.port));
  const prefix = `/api/v1/workspaces/${workspaceId}/stats`;
  await page.route("**/api/v1/**", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname === `${prefix}/completed-activity`) {
      await route.fulfill({ json: completedActivity });
    } else if (pathname === `${prefix}/daily-activity`) {
      await route.fulfill({ json: dailyActivity });
    } else {
      await route.continue();
    }
  });
  const completedRead = waitForHttp(page, "GET", new RegExp(`^${prefix}/completed-activity$`));
  const dailyRead = waitForHttp(page, "GET", new RegExp(`^${prefix}/daily-activity$`));
  await page.goto(`${backend.frontendUrl}/stats?workspaceId=${workspaceId}`);
  const [completedResponse, dailyResponse] = await Promise.all([completedRead, dailyRead]);
  expect(completedResponse.ok()).toBe(true);
  expect(await completedResponse.json()).toEqual(completedActivity);
  expect(dailyResponse.ok()).toBe(true);
  expect(await dailyResponse.json()).toEqual(dailyActivity);
  expect(await page.evaluate(() => navigator.language)).toBe("en-US");
  expect(await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone)).toBe(
    timezoneId,
  );
  const card = page.locator('#completed [data-slot="card"]').filter({
    has: page.getByText("Completed Tasks Over Time", { exact: true }),
  });
  await expect(card).toHaveCount(1);
  await expect(card).toBeVisible();
  return card;
}

export async function assertCalendarBuckets(page: Page, card: Locator, touch = false) {
  for (const bucket of buckets) {
    const control = card.getByRole("button", { name: bucket.name, exact: true });
    await expect(control).toHaveCount(1);
    if (touch) await control.tap();
    else await control.click();
    const bars = card.locator('.h-32 > [data-slot="tooltip-trigger"]');
    await expect(bars).toHaveCount(bucket.labels.length);
    const endpoints = card.locator(".justify-between > span");
    await expect(endpoints).toHaveText([bucket.labels[0], bucket.labels[bucket.labels.length - 1]]);
    for (const [index, label] of bucket.labels.entries()) {
      const bar = bars.nth(index);
      expect(await bar.evaluate((element) => (element as HTMLElement).style.height)).toBe(
        `${bucket.heights[index]}%`,
      );
      if (!touch) {
        await page.mouse.move(0, 0);
        await bar.hover();
        const tooltip = page.locator('[data-slot="tooltip-content"]:not([data-state="closed"])');
        await expect(tooltip).toHaveCount(1);
        await expect(tooltip.locator(":scope > .font-medium")).toHaveText(label);
        await expect(tooltip.locator(":scope > .text-muted-foreground")).toHaveText(
          `${bucket.counts[index]} completed`,
        );
      }
    }
  }
}

export async function assertActivityCalendarDate(page: Page) {
  const activity = page.locator('#activity [data-slot="card"]').filter({
    has: page.getByText(/^Activity \(/),
  });
  await expect(activity).toHaveCount(1);
  const days = activity.locator('[data-slot="tooltip-trigger"]');
  await expect(days).toHaveCount(5);
  await expect(activity.getByText("Jan", { exact: true })).toHaveCount(1);
  await page.mouse.move(0, 0);
  await days.nth(0).hover();
  const tooltip = page.locator('[data-slot="tooltip-content"]:not([data-state="closed"])');
  await expect(tooltip).toHaveCount(1);
  await expect(tooltip.locator(":scope > .font-medium")).toHaveText("Jan 1");
  await expect(tooltip.locator(":scope > .text-muted-foreground")).toHaveText(
    "2 turns, 3 messages",
  );
}

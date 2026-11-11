import { expect, type APIResponse, type Locator, type Page } from "@playwright/test";
import type { RetentionStatus } from "../../lib/types/system";
import { waitForHttp } from "./causal-waits";

const retentionPath = "/api/v1/system/retention";
const storagePath = "/settings/system/storage?tab=office-retention";
async function activate(control: Locator, phone: boolean) {
  await expect(control).toHaveCount(1);
  if (phone) await control.tap();
  else await control.click();
}
async function expand(page: Page, phone: boolean) {
  await activate(page.getByTestId("retention-advanced-settings").locator("summary"), phone);
}
async function save(page: Page, batch: string, phone: boolean) {
  await page.getByTestId("retention-batch-limit").fill(batch);
  const put = waitForHttp(page, "PUT", /\/api\/v1\/system\/retention$/);
  await activate(
    page.getByTestId("settings-floating-save").getByRole("button", { name: "Save changes" }),
    phone,
  );
  expect((await put).status()).toBe(200);
  await expect(page.getByTestId("settings-floating-save")).not.toHaveAttribute(
    "data-status",
    "saving",
  );
}
async function reenter(page: Page, phone: boolean) {
  const origin = await page.evaluate(() => performance.timeOrigin);
  if (phone) {
    await activate(page.getByRole("link", { name: "Settings", exact: true }), true);
    await expect(page.getByTestId("settings-index")).toBeVisible();
    const system = page
      .getByTestId("settings-index")
      .getByRole("button", { name: "Expand System", exact: true });
    if (await system.isVisible()) await system.tap();
  } else {
    await activate(page.locator('a[href="/settings/system/status"]:visible'), false);
    await expect(page.getByTestId("retention-settings")).toHaveCount(0);
  }
  await activate(page.locator('a[href="/settings/system/storage"]:visible'), phone);
  await expect(page).toHaveURL(/\/settings\/system\/storage/);
  await activate(page.getByRole("tab", { name: "Office retention", exact: true }), phone);
  await expand(page, phone);
  expect(await page.evaluate(() => performance.timeOrigin)).toBe(origin);
}

// @covers AC-OFFICE-RUN-HISTORY-RETENTION-004.10
export async function proveRetentionRefreshOrdering(page: Page, phone: boolean) {
  const originalResponse = await page.request.get(retentionPath);
  expect(originalResponse.ok()).toBe(true);
  const original: RetentionStatus = await originalResponse.json();
  const held: Array<{ release: () => void; done: Promise<void>; snapshot: RetentionStatus }> = [];
  let hold = false;
  let puts = 0;
  const countWrites = (request: import("@playwright/test").Request) => {
    if (request.method() === "PUT" && new URL(request.url()).pathname === retentionPath) puts++;
  };
  page.on("request", countWrites);
  await page.route(`**${retentionPath}`, async (route) => {
    if (route.request().method() !== "GET" || !hold) return route.continue();
    const response: APIResponse = await route.fetch();
    if (!hold) return route.fulfill({ response });
    const snapshot: RetentionStatus = await response.json();
    let release!: () => void;
    const ready = new Promise<void>((resolve) => {
      release = resolve;
    });
    let finished!: () => void;
    const done = new Promise<void>((resolve) => {
      finished = resolve;
    });
    held.push({ release, done, snapshot });
    try {
      await ready;
      await route.fulfill({ response });
    } finally {
      finished();
    }
  });
  try {
    // Only batch limits change. No sweep, deletion, preview or scheduling action runs.
    await page.goto(storagePath);
    await expand(page, phone);
    hold = true;
    await save(page, "6000", phone);
    await expect.poll(() => held.length).toBe(1);
    expect(held[0].snapshot.settings.batch_limit).toBe(6000);
    await reenter(page, phone);
    await expect(page.getByTestId("retention-batch-limit")).toBeEnabled();
    await save(page, "7000", phone);
    await expect.poll(() => held.length).toBe(2);
    expect(held[1].snapshot.settings.batch_limit).toBe(7000);
    held[1].release();
    await held[1].done;
    await expect(page.getByTestId("retention-batch-limit")).toBeEnabled();
    const staleResponse = page.waitForResponse(
      (r) => new URL(r.url()).pathname === retentionPath && r.request().method() === "GET",
    );
    held[0].release();
    await held[0].done;
    await (await staleResponse).finished();
    await page.evaluate(
      () =>
        new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        ),
    );
    await expect(page.getByTestId("retention-batch-limit")).toHaveValue("7000");
    await expect(page.getByTestId("settings-floating-save")).not.toHaveAttribute(
      "data-status",
      "dirty",
    );
    expect((await (await page.request.get(retentionPath)).json()).settings.batch_limit).toBe(7000);
    await page.getByTestId("retention-batch-limit").fill("8000");
    await activate(
      page
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Reset", exact: true }),
      phone,
    );
    await expect(page.getByTestId("retention-batch-limit")).toHaveValue("7000");
    await expect(page.getByTestId("settings-floating-save")).not.toBeVisible();
    expect(puts).toBe(2);
    hold = false;
    await save(page, "8000", phone);
    await expect(page.getByTestId("retention-batch-limit")).toBeEnabled();
    expect((await (await page.request.get(retentionPath)).json()).settings.batch_limit).toBe(8000);
    await page.reload();
    await expand(page, phone);
    await expect(page.getByTestId("retention-batch-limit")).toHaveValue("8000");
  } finally {
    hold = false;
    for (const pending of held) pending.release();
    await Promise.all(held.map((pending) => pending.done));
    await page.unroute(`**${retentionPath}`);
    page.off("request", countWrites);
    const restored = await page.request.put(retentionPath, { data: original.settings });
    expect(restored.ok()).toBe(true);
    expect((await (await page.request.get(retentionPath)).json()).settings).toEqual(
      original.settings,
    );
  }
}

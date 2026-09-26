import { expect, type BrowserContext } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { watchWs } from "../../helpers/causal-waits";

/**
 * Relative "Last seen" on the account security page (desktop + cross-tab).
 *
 * Runs in the `auth` project with the worker backend restarted to require
 * auth and use its own database. Serial: auth setup is single-shot per
 * database and the worker DB survives restarts, so the file owns its own
 * `KANDEV_DATABASE_PATH` (the users-self-actions isolation pattern) and
 * restores the baseline backend in afterAll.
 */
const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };
const SECURITY_PATH = "/settings/account/security";
const CURRENT_SESSION_ROW = '[data-testid="account-sessions-row"][data-current-session="true"]';

/** Fetches the user's current last_seen_display setting from the settings API. */
async function readLastSeenDisplay(ctx: BrowserContext, baseUrl: string): Promise<string> {
  const res = await ctx.request.get(`${baseUrl}/api/v1/user/settings`);
  expect(res.ok(), await res.text()).toBeTruthy();
  const body = (await res.json()) as { settings: { last_seen_display?: string } };
  return body.settings.last_seen_display ?? "absolute";
}

/** Persists the given last_seen_display value through the settings API. */
async function restoreLastSeenDisplay(
  ctx: BrowserContext,
  baseUrl: string,
  value: string,
): Promise<void> {
  const res = await ctx.request.patch(`${baseUrl}/api/v1/user/settings`, {
    data: { last_seen_display: value },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

test.describe.serial("relative last seen (desktop)", () => {
  test.beforeAll(async ({ browser, backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-relative-last-seen.db"),
    });
    const setupContext = await browser.newContext({ baseURL: backend.frontendUrl });
    try {
      await setupAdmin(setupContext, backend.baseUrl, ADMIN);
    } finally {
      await setupContext.close();
    }
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("switches Last seen to relative time and shows a relative label with an absolute tooltip", async ({
    browser,
    backend,
  }) => {
    const ctx = await browser.newContext({ baseURL: backend.frontendUrl });
    try {
      await login(ctx, backend.baseUrl, ADMIN);
      let original = "absolute";
      try {
        original = await readLastSeenDisplay(ctx, backend.baseUrl);
        // Pin a known absolute baseline so selecting Relative time is a real
        // transition and PATCH, not a no-op on a pre-existing relative value.
        await restoreLastSeenDisplay(ctx, backend.baseUrl, "absolute");

        const page = await ctx.newPage();
        await page.goto(SECURITY_PATH);
        await expect(page.getByTestId("last-seen-relative")).toHaveCount(0);

        const select = page.getByTestId("last-seen-display-select");
        await expect(select).toBeVisible({ timeout: 15_000 });

        await select.click();
        await page.getByRole("listbox").getByRole("option", { name: "Relative time" }).click();
        await page.getByRole("button", { name: "Save changes" }).click();

        const relative = page.locator(CURRENT_SESSION_ROW).getByTestId("last-seen-relative");
        await expect(relative).toBeVisible({ timeout: 15_000 });

        // Hover reveals the absolute timestamp in a tooltip, matching the
        // trigger's accessible name/native title exactly.
        const absolute = await relative.getAttribute("title");
        expect(absolute).toBeTruthy();
        await relative.hover();
        const tooltip = page.getByRole("tooltip");
        await expect(tooltip).toBeVisible();
        await expect(tooltip).toHaveText(absolute!);

        // The choice persists to the user-settings API.
        await expect
          .poll(() => readLastSeenDisplay(ctx, backend.baseUrl), { timeout: 15_000 })
          .toBe("relative");
      } finally {
        await restoreLastSeenDisplay(ctx, backend.baseUrl, original);
      }
    } finally {
      await ctx.close();
    }
  });

  test("syncs a Last seen change to another tab over WebSocket without a reload", async ({
    browser,
    backend,
  }) => {
    const ctxA = await browser.newContext({ baseURL: backend.frontendUrl });
    const ctxB = await browser.newContext({ baseURL: backend.frontendUrl });
    try {
      await login(ctxA, backend.baseUrl, ADMIN);
      await login(ctxB, backend.baseUrl, ADMIN);
      let original = "absolute";
      try {
        original = await readLastSeenDisplay(ctxA, backend.baseUrl);
        // Pin a known absolute baseline so tab A's pre-mutation display is
        // established: the relative assertion below can only pass via a real
        // WS event, not a stale pre-existing value.
        await restoreLastSeenDisplay(ctxA, backend.baseUrl, "absolute");

        const pageA = await ctxA.newPage();
        // watchWs only observes sockets opened after it is armed, and the
        // subscription wait itself must be armed before navigation.
        const watcher = watchWs(pageA);
        const subscriptionAck = watcher.waitForResponse("user.subscribe");
        await pageA.goto(SECURITY_PATH);
        await subscriptionAck;
        await expect(pageA.getByTestId("last-seen-relative")).toHaveCount(0);

        // Arm the WS event wait before tab B mutates; frames are not buffered,
        // so a wait armed after the mutation could miss the push entirely.
        const settingsUpdated = watcher.waitForEvent("user.settings.updated");

        const pageB = await ctxB.newPage();
        await pageB.goto(SECURITY_PATH);
        await expect(pageB.getByTestId("last-seen-relative")).toHaveCount(0);
        await pageB.getByTestId("last-seen-display-select").click();
        await pageB.getByRole("listbox").getByRole("option", { name: "Relative time" }).click();
        await pageB.getByRole("button", { name: "Save changes" }).click();
        await expect(
          pageB.locator(CURRENT_SESSION_ROW).getByTestId("last-seen-relative"),
        ).toBeVisible({
          timeout: 15_000,
        });

        // Tab A observes the change over WebSocket, without a reload.
        await settingsUpdated;
        await expect(
          pageA.locator(CURRENT_SESSION_ROW).getByTestId("last-seen-relative"),
        ).toBeVisible();
      } finally {
        await restoreLastSeenDisplay(ctxA, backend.baseUrl, original);
      }
    } finally {
      await ctxA.close();
      await ctxB.close();
    }
  });
});

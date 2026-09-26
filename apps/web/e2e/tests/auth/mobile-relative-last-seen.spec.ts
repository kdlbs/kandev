import { devices, expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";

/**
 * Mobile parity for the relative "Last seen" option: the select is operated
 * with touch-native `tap()` (mouse-semantics clicks would not prove touch),
 * relative labels render without hover, and the trigger/option meet the 44px
 * active-dimension touch target at the Pixel 5 viewport.
 *
 * Named `mobile-*` so the `mobile-chrome` project routes it away from the
 * desktop `auth` project. Manual contexts do not inherit project device
 * options, so the Pixel 5 viewport is spread explicitly.
 */
const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };
const SECURITY_PATH = "/settings/account/security";
const CURRENT_SESSION_ROW = '[data-testid="account-sessions-row"][data-current-session="true"]';

test.describe.serial("relative last seen (mobile)", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-mobile-relative-last-seen.db"),
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("operates the Last seen select with touch and renders relative labels without hover", async ({
    browser,
    backend,
  }) => {
    const ctx = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    try {
      await setupAdmin(ctx, backend.baseUrl, ADMIN);
      await login(ctx, backend.baseUrl, ADMIN);
      let original = "absolute";
      try {
        const originalRes = await ctx.request.get(`${backend.baseUrl}/api/v1/user/settings`);
        expect(originalRes.ok(), await originalRes.text()).toBeTruthy();
        const originalBody = (await originalRes.json()) as {
          settings: { last_seen_display?: string };
        };
        original = originalBody.settings.last_seen_display ?? "absolute";
        // Pin a known absolute baseline so the relative transition below is a
        // real persisted change, not a pre-existing value.
        const baselineRes = await ctx.request.patch(`${backend.baseUrl}/api/v1/user/settings`, {
          data: { last_seen_display: "absolute" },
        });
        expect(baselineRes.ok(), await baselineRes.text()).toBeTruthy();

        const page = await ctx.newPage();
        // Manual contexts do not inherit project device options; pin the width.
        expect((await page.viewportSize())?.width).toBe(393);

        await page.goto(SECURITY_PATH);
        await expect(page.getByTestId("last-seen-relative")).toHaveCount(0);

        const trigger = page.getByTestId("last-seen-display-select");
        await expect(trigger).toBeVisible({ timeout: 15_000 });

        // The trigger meets the 44px active-dimension touch target.
        const triggerBox = await trigger.boundingBox();
        expect(triggerBox).not.toBeNull();
        expect(triggerBox!.height).toBeGreaterThanOrEqual(44);

        await trigger.tap();
        const option = page.getByRole("listbox").getByRole("option", { name: "Relative time" });
        await expect(option).toBeVisible();
        // The dropdown entrance animation (zoom-in-95 over 100ms) scales the
        // whole content, so a one-shot boundingBox() can measure mid-flight
        // and read ~42px. Poll until the box settles at the real size before
        // asserting the 44px active-dimension touch target.
        await expect
          .poll(async () => Math.round((await option.boundingBox())?.height ?? 0))
          .toBeGreaterThanOrEqual(44);
        await option.tap();
        await page.getByRole("button", { name: "Save changes" }).tap();

        // Relative labels render without hover, with no horizontal overflow.
        const relative = page.locator(CURRENT_SESSION_ROW).getByTestId("last-seen-relative");
        await expect(relative).toBeVisible({ timeout: 15_000 });
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
        );
        expect(overflow).toBe(false);

        // The absolute stamp stays reachable through an explicit touch drawer.
        const absolute = await relative.getAttribute("title");
        expect(absolute).toBeTruthy();
        await expect(relative).toHaveAttribute("aria-label", absolute!);
        await relative.tap();
        const absoluteDetails = page.getByTestId("last-seen-absolute");
        await expect(absoluteDetails).toBeVisible();
        await expect(absoluteDetails).toHaveText(absolute!);
      } finally {
        const res = await ctx.request.patch(`${backend.baseUrl}/api/v1/user/settings`, {
          data: { last_seen_display: original },
        });
        expect(res.ok(), await res.text()).toBeTruthy();
      }
    } finally {
      await ctx.close();
    }
  });
});

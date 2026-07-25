import { expect } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { createInviteToken, login } from "../../helpers/auth";

/**
 * Not an assertion suite — captures screenshots of every auth surface into
 * e2e/.auth-screenshots/ for manual review. Runs in the `auth` project
 * (backend restarted with auth required). Serial: shares the admin created
 * by the setup shot.
 */
const SHOT_DIR = path.join(__dirname, "..", "..", ".auth-screenshots");

test.describe.serial("auth screenshots", () => {
  const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };
  const MEMBER = { email: "sam@demo.dev", password: "memberpass123", displayName: "Sam Member" };

  test.beforeAll(async ({ backend }) => {
    fs.mkdirSync(SHOT_DIR, { recursive: true });
    // The features.auth flag turns authentication on (setup mode) and reveals
    // the admin surfaces.
    await backend.restart({ KANDEV_FEATURES_AUTH: "true" });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  async function shot(page: import("@playwright/test").Page, name: string) {
    await page.screenshot({ path: path.join(SHOT_DIR, `${name}.png`), fullPage: true });
  }

  test("01 setup wizard", async ({ browser, backend }) => {
    const ctx = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 1280, height: 900 },
    });
    const page = await ctx.newPage();
    await page.goto("/");
    await expect(page.getByTestId("setup-email")).toBeVisible({ timeout: 15_000 });
    await page.getByTestId("setup-email").fill(ADMIN.email);
    await page.getByTestId("setup-display-name").fill(ADMIN.displayName);
    await page.getByTestId("setup-password").fill(ADMIN.password);
    await shot(page, "01-setup-wizard");
    await page.getByTestId("setup-submit").click();
    await page.waitForURL("**/");
    await expect(page.getByTestId("setup-email")).toHaveCount(0, { timeout: 15_000 });
    await page.waitForTimeout(1500);
    await shot(page, "02-app-after-setup");
    await ctx.close();
  });

  test("02 login page", async ({ browser, backend }) => {
    const ctx = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 1280, height: 900 },
    });
    const page = await ctx.newPage();
    await page.goto("/");
    await expect(page.getByTestId("login-email")).toBeVisible({ timeout: 15_000 });
    await page.getByTestId("login-email").fill(ADMIN.email);
    await page.getByTestId("login-password").fill(ADMIN.password);
    await shot(page, "03-login");
    await ctx.close();
  });

  test("03 feature toggles (auth control)", async ({ browser, backend }) => {
    const ctx = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 1280, height: 900 },
    });
    await login(ctx, backend.baseUrl, ADMIN);
    const page = await ctx.newPage();
    // Authentication is enabled/disabled from the Feature Toggles page — the
    // "Authentication & users" flag — not a dedicated settings section.
    await page.goto("/settings/system/feature-toggles");
    await expect(page.getByTestId("feature-toggles-settings")).toBeVisible({ timeout: 15_000 });
    await shot(page, "04-feature-toggles");
    await ctx.close();
  });

  test("04 users table + dialogs", async ({ browser, backend }) => {
    const ctx = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 1280, height: 900 },
    });
    await login(ctx, backend.baseUrl, ADMIN);
    // Seed a member so the table has more than one row.
    const token = await createInviteToken(ctx, backend.baseUrl, {
      email: MEMBER.email,
      role: "member",
    });
    const memberCtx = await browser.newContext({ baseURL: backend.frontendUrl });
    await memberCtx.request.post(`${backend.baseUrl}/api/v1/auth/invites/accept`, {
      data: {
        token,
        email: MEMBER.email,
        password: MEMBER.password,
        display_name: MEMBER.displayName,
      },
    });
    await memberCtx.close();

    const page = await ctx.newPage();
    await page.goto("/settings/system/users");
    await expect(page.getByTestId("users-table")).toBeVisible({ timeout: 15_000 });
    await shot(page, "05-settings-users");

    await page.getByTestId("users-table-invite").click();
    await expect(page.getByTestId("invite-dialog")).toBeVisible();
    await page.getByTestId("invite-dialog-email").fill("newhire@demo.dev");
    await page.getByTestId("invite-dialog-submit").click();
    await expect(page.getByTestId("invite-dialog-url")).toBeVisible({ timeout: 10_000 });
    await shot(page, "06-invite-dialog");
    await page.keyboard.press("Escape");

    await page.getByTestId("users-table-create").click();
    await expect(page.getByTestId("create-user-dialog")).toBeVisible();
    await page.getByTestId("create-user-email").fill("contractor@demo.dev");
    await page.getByTestId("create-user-display-name").fill("Casey Contractor");
    await page.getByTestId("create-user-password").fill("contractorpass123");
    await shot(page, "07-create-user-dialog");
    await ctx.close();
  });

  test("05 account security + tokens", async ({ browser, backend }) => {
    const ctx = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 1280, height: 900 },
    });
    await login(ctx, backend.baseUrl, ADMIN);
    const page = await ctx.newPage();

    await page.goto("/settings/account/security");
    await expect(page.getByTestId("account-security-password-card")).toBeVisible({
      timeout: 15_000,
    });
    await shot(page, "08-account-security");

    await page.goto("/settings/account/tokens");
    await expect(page.getByTestId("api-tokens-card")).toBeVisible({ timeout: 15_000 });
    await page.getByTestId("api-tokens-create").click();
    await expect(page.getByTestId("api-tokens-mint-dialog")).toBeVisible();
    await page.getByTestId("api-tokens-name").fill("ci-pipeline");
    await page.getByTestId("api-tokens-mint-submit").click();
    await expect(page.getByTestId("api-tokens-raw-value")).toBeVisible({ timeout: 10_000 });
    await shot(page, "09-account-token-minted");
    await ctx.close();
  });

  test("06 invite accept page", async ({ browser, backend }) => {
    const adminCtx = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(adminCtx, backend.baseUrl, ADMIN);
    const token = await createInviteToken(adminCtx, backend.baseUrl, {
      email: "invitee@demo.dev",
      role: "member",
    });
    await adminCtx.close();

    const ctx = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 1280, height: 900 },
    });
    const page = await ctx.newPage();
    await page.goto(`/invite?token=${token}`);
    await expect(page.getByTestId("invite-accept-submit")).toBeVisible({ timeout: 15_000 });
    await page.getByTestId("invite-password").fill("inviteepass123");
    await page.getByTestId("invite-display-name").fill("Ivy Invitee");
    await shot(page, "10-invite-accept");
    await ctx.close();
  });

  test("07 member segregated view", async ({ browser, backend }) => {
    // Admin creates a private workspace; member logs in and sees only their own.
    const adminCtx = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(adminCtx, backend.baseUrl, ADMIN);
    await adminCtx.request.post(`${backend.baseUrl}/api/v1/workspaces`, {
      data: { name: "Ada's Private Workspace" },
    });
    await adminCtx.close();

    const ctx = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 1280, height: 900 },
    });
    await login(ctx, backend.baseUrl, MEMBER);
    const page = await ctx.newPage();
    await page.goto("/");
    await page.waitForTimeout(2000);
    await shot(page, "11-member-app");
    // Dismiss the first-run onboarding wizard (it overlays the sidebar), then
    // hover the current-user chip to verify the border (not accent-fill) hover.
    const skip = page.getByRole("button", { name: /skip/i });
    if (await skip.isVisible().catch(() => false)) {
      await skip.click();
      await page.waitForTimeout(400);
    }
    const chip = page.getByTestId("current-user-chip");
    await chip.hover();
    await page.waitForTimeout(300);
    await chip.screenshot({ path: path.join(SHOT_DIR, "12-user-chip-hover.png") });
    await ctx.close();
  });
});

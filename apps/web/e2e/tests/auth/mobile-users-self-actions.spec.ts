import { devices, expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { responseUserId } from "./users-auth-helpers";

/**
 * Mobile parity for the self-actions guard: the Users page must grey out the
 * role/status toggles on an active-admin row when no other active admin
 * exists on the Pixel 5 path too. The component and state derivation are
 * shared with desktop; this spec proves the rendered disabled state at the
 * mobile viewport.
 *
 * Runs in the `mobile-chrome` project (routed away from the desktop `auth`
 * project via its testIgnore) and restarts the worker backend with auth on
 * and its own database, mirroring users-self-actions.spec.ts. afterAll
 * restarts to the fixture baseline.
 */
const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };
const MEMBER = { email: "sam@demo.dev", password: "memberpass123", displayName: "Sam Member" };
const DISABLED_ADMIN = {
  email: "carol@demo.dev",
  password: "carolpass123",
  displayName: "Carol Admin (off)",
};
const SECOND_ADMIN = { email: "bob@demo.dev", password: "bobpass123", displayName: "Bob Admin" };

test.describe.serial("users self-actions guard (mobile)", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-mobile-self-actions.db"),
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("own-row toggles track the last-admin guard on mobile", async ({ browser, backend }) => {
    // Manual contexts do not inherit the project's device options; spread
    // them explicitly so this spec actually exercises the mobile viewport.
    const ctx = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    await setupAdmin(ctx, backend.baseUrl, ADMIN);
    await login(ctx, backend.baseUrl, ADMIN);

    const meRes = await ctx.request.get(`${backend.baseUrl}/api/v1/auth/me`);
    expect(meRes.ok(), await meRes.text()).toBeTruthy();
    const adminId = responseUserId(await meRes.json());

    const memberRes = await ctx.request.post(`${backend.baseUrl}/api/v1/users`, {
      data: {
        email: MEMBER.email,
        password: MEMBER.password,
        display_name: MEMBER.displayName,
        role: "member",
      },
    });
    expect(memberRes.status(), await memberRes.text()).toBe(201);
    const memberId = responseUserId(await memberRes.json());

    const disabledRes = await ctx.request.post(`${backend.baseUrl}/api/v1/users`, {
      data: {
        email: DISABLED_ADMIN.email,
        password: DISABLED_ADMIN.password,
        display_name: DISABLED_ADMIN.displayName,
        role: "member",
      },
    });
    expect(disabledRes.status(), await disabledRes.text()).toBe(201);
    const disabledAdminId = responseUserId(await disabledRes.json());
    expect(
      (
        await ctx.request.patch(`${backend.baseUrl}/api/v1/users/${disabledAdminId}`, {
          data: { role: "admin" },
        })
      ).status(),
    ).toBe(200);
    expect(
      (
        await ctx.request.patch(`${backend.baseUrl}/api/v1/users/${disabledAdminId}`, {
          data: { status: "disabled" },
        })
      ).status(),
    ).toBe(200);

    const page = await ctx.newPage();
    // Guard against the manual-context device regression: the assertions
    // below must run on the Pixel 5 width (393 CSS px; headless mobile
    // emulation trims the height, so only the width is pinned).
    expect((await page.viewportSize())?.width).toBe(393);
    await page.goto("/settings/system/users");
    const ownRow = page.locator(`[data-user-id="${adminId}"]`);
    const memberRow = page.locator(`[data-user-id="${memberId}"]`);
    const disabledAdminRow = page.locator(`[data-user-id="${disabledAdminId}"]`);
    await expect(ownRow).toBeVisible({ timeout: 15_000 });

    // Sole active admin: own row disabled; member and disabled-admin rows
    // are not gated by the last-admin guard.
    await expect(ownRow.getByTestId("users-table-toggle-role")).toBeDisabled();
    await expect(ownRow.getByTestId("users-table-toggle-status")).toBeDisabled();
    await expect(memberRow.getByTestId("users-table-toggle-role")).toBeEnabled();
    await expect(memberRow.getByTestId("users-table-toggle-status")).toBeEnabled();
    await expect(disabledAdminRow.getByTestId("users-table-toggle-role")).toBeEnabled();
    await expect(disabledAdminRow.getByTestId("users-table-toggle-status")).toBeEnabled();

    // A second active admin lifts the guard on every active-admin row.
    const secondRes = await ctx.request.post(`${backend.baseUrl}/api/v1/users`, {
      data: {
        email: SECOND_ADMIN.email,
        password: SECOND_ADMIN.password,
        display_name: SECOND_ADMIN.displayName,
        role: "admin",
      },
    });
    expect(secondRes.status(), await secondRes.text()).toBe(201);
    const secondAdminId = responseUserId(await secondRes.json());

    await page.reload();
    const secondAdminRow = page.locator(`[data-user-id="${secondAdminId}"]`);
    await expect(ownRow.getByTestId("users-table-toggle-role")).toBeEnabled();
    await expect(ownRow.getByTestId("users-table-toggle-status")).toBeEnabled();
    await expect(secondAdminRow.getByTestId("users-table-toggle-role")).toBeEnabled();
    await expect(secondAdminRow.getByTestId("users-table-toggle-status")).toBeEnabled();

    await ctx.close();
  });
});

import { expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { responseUserId } from "./users-auth-helpers";

/**
 * The Users page must grey out the role/status toggles on an active-admin row
 * when no other active admin exists, mirroring the backend's existing
 * last-admin guard (ErrLastAdmin -> 409). The backend itself is unchanged:
 * self-demotion stays allowed while another active admin exists.
 *
 * Runs in the `auth` project (backend restarted with auth required). Serial:
 * shares the admin and users created inside the single test.
 *
 * Isolation: the worker-scoped backend fixture and `restart()` preserve the
 * SQLite DB, and auth setup is single-shot per database. This spec restarts
 * with its own database path so the full auth project stays deterministic in
 * either file order (auth-screenshots.spec.ts sets up the same admin email on
 * the baseline database).
 */
const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };
const MEMBER = { email: "sam@demo.dev", password: "memberpass123", displayName: "Sam Member" };
const DISABLED_ADMIN = {
  email: "carol@demo.dev",
  password: "carolpass123",
  displayName: "Carol Admin (off)",
};
const SECOND_ADMIN = { email: "bob@demo.dev", password: "bobpass123", displayName: "Bob Admin" };

test.describe.serial("users self-actions guard", () => {
  test.beforeAll(async ({ backend }) => {
    // The features.auth flag turns authentication on (setup mode) and reveals
    // the admin surfaces. A per-file database keeps setup single-shot from
    // colliding with other auth specs sharing the worker.
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-auth-self-actions.db"),
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("own-row toggles track the last-admin guard; backend unchanged", async ({
    browser,
    backend,
  }) => {
    const ctx = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(ctx, backend.baseUrl, ADMIN);
    await login(ctx, backend.baseUrl, ADMIN);

    // The setup helper does not return the user; fetch the admin id.
    const meRes = await ctx.request.get(`${backend.baseUrl}/api/v1/auth/me`);
    expect(meRes.ok(), await meRes.text()).toBeTruthy();
    const adminId = responseUserId(await meRes.json());

    // A member row provides the enabled baseline.
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

    const page = await ctx.newPage();
    await page.goto("/settings/system/users");
    const ownRow = page.locator(`[data-user-id="${adminId}"]`);
    const memberRow = page.locator(`[data-user-id="${memberId}"]`);
    await expect(ownRow).toBeVisible({ timeout: 15_000 });

    // Sole active admin: the own row's demote/disable toggles are disabled,
    // exactly where the backend would reject the action.
    await expect(ownRow.getByTestId("users-table-toggle-role")).toBeDisabled();
    await expect(ownRow.getByTestId("users-table-toggle-status")).toBeDisabled();
    // A member row is never gated by the last-admin guard.
    await expect(memberRow.getByTestId("users-table-toggle-role")).toBeEnabled();
    await expect(memberRow.getByTestId("users-table-toggle-status")).toBeEnabled();

    // Existing guard still enforced at the API: the sole admin cannot
    // self-demote.
    const selfDemote = await ctx.request.patch(`${backend.baseUrl}/api/v1/users/${adminId}`, {
      data: { role: "member" },
    });
    expect(selfDemote.status(), await selfDemote.text()).toBe(409);

    // A disabled admin is NOT an active admin: the guard does not apply to
    // its toggles, and counting it must not lift the sole active admin's
    // disablement (regression: dropping `status === "active"` in either the
    // count or the per-row condition would flip these two assertions).
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
    const promote = await ctx.request.patch(`${backend.baseUrl}/api/v1/users/${disabledAdminId}`, {
      data: { role: "admin" },
    });
    expect(promote.status(), await promote.text()).toBe(200);
    const disable = await ctx.request.patch(`${backend.baseUrl}/api/v1/users/${disabledAdminId}`, {
      data: { status: "disabled" },
    });
    expect(disable.status(), await disable.text()).toBe(200);

    await page.reload();
    const disabledAdminRow = page.locator(`[data-user-id="${disabledAdminId}"]`);
    await expect(ownRow.getByTestId("users-table-toggle-role")).toBeDisabled();
    await expect(ownRow.getByTestId("users-table-toggle-status")).toBeDisabled();
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

    // No new backend behavior: self-demotion is allowed again once another
    // active admin exists.
    const selfDemoteAfter = await ctx.request.patch(`${backend.baseUrl}/api/v1/users/${adminId}`, {
      data: { role: "member" },
    });
    expect(selfDemoteAfter.status(), await selfDemoteAfter.text()).toBe(200);

    await ctx.close();
  });
});

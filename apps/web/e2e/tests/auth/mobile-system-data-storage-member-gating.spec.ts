import { devices, expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import {
  createMember,
  createSnapshotAsAdmin,
  DATA_STORAGE_ROUTE,
  expectMemberApiGating,
  GATING_ADMIN,
  GATING_MEMBER,
} from "./data-storage-gating-helpers";

/**
 * Mobile parity for the Data & storage member gate. The gating adds no new
 * composition: it removes controls from the backups table and disables the
 * storage actions inside surfaces that already have shipped phone layouts
 * (see tests/system/mobile-storage-maintenance.spec.ts). What is
 * viewport-specific is the consequence: dropping the backups Actions column
 * changes the table's column count, and a table is the surface most likely to
 * push a phone into horizontal scroll. This spec proves the member view is
 * complete, inert, and contained at Pixel 5 width.
 *
 * Runs in the `mobile-chrome` project (routed away from the desktop `auth`
 * project via its testIgnore) and restarts the worker backend with auth on
 * and its own database, mirroring system-data-storage-member-gating.spec.ts.
 * afterAll restarts to the fixture baseline.
 */
test.describe.serial("Data & storage member gating (mobile)", () => {
  let snapshotName = "";

  test.beforeAll(async ({ backend, browser }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-mobile-data-storage.db"),
    });
    const adminContext = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    await setupAdmin(adminContext, backend.baseUrl, GATING_ADMIN);
    await login(adminContext, backend.baseUrl, GATING_ADMIN);
    await createMember(adminContext, backend.baseUrl);
    snapshotName = await createSnapshotAsAdmin(adminContext, backend.baseUrl);
    await adminContext.close();
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("a member gets the read-only view, inert and contained, on a phone", async ({
    browser,
    backend,
  }) => {
    // Manual contexts do not inherit the project's device options; spread
    // them explicitly so this spec actually exercises the mobile viewport.
    const ctx = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    await login(ctx, backend.baseUrl, GATING_MEMBER);
    await expectMemberApiGating(ctx, backend.baseUrl, snapshotName);

    const page = await ctx.newPage();
    expect((await page.viewportSize())?.width).toBe(393);
    await page.goto(DATA_STORAGE_ROUTE);

    await expect(page.getByTestId("system-backups-name").first()).toHaveText(snapshotName, {
      timeout: 15_000,
    });
    await expect(page.getByTestId("system-backups-admin-only")).toBeVisible();
    await expect(page.getByTestId("system-backups-create")).toHaveCount(0);
    await expect(page.getByTestId("system-backups-download")).toHaveCount(0);
    await expect(page.getByTestId("system-backups-restore")).toHaveCount(0);
    await expect(page.getByTestId("system-backups-delete")).toHaveCount(0);

    const analyze = page.getByTestId("storage-analyze");
    await expect(analyze).toBeDisabled();
    await expect(page.getByTestId("storage-run-now")).toBeDisabled();

    // The reason must stay reachable by touch rather than hover-only: the
    // disabled control keeps a touch-sized target so its tooltip trigger is
    // usable on a phone.
    const analyzeBox = await analyze.boundingBox();
    expect(analyzeBox).not.toBeNull();
    expect(analyzeBox!.height).toBeGreaterThanOrEqual(44);

    // The narrower table must not push the document into horizontal scroll.
    await expect
      .poll(() =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);

    await ctx.close();
  });

  test("an admin still gets every control on a phone", async ({ browser, backend }) => {
    const ctx = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    await login(ctx, backend.baseUrl, GATING_ADMIN);

    const page = await ctx.newPage();
    expect((await page.viewportSize())?.width).toBe(393);
    await page.goto(DATA_STORAGE_ROUTE);

    await expect(page.getByTestId("system-backups-create")).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId("system-backups-delete").first()).toBeVisible();
    await expect(page.getByTestId("system-backups-admin-only")).toHaveCount(0);
    await expect(page.getByTestId("storage-analyze")).toBeEnabled();

    await expect
      .poll(() =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      )
      .toBe(true);

    await ctx.close();
  });
});

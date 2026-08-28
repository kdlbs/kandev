import { expect } from "@playwright/test";
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
 * System backups and storage maintenance act on the whole install: a snapshot
 * is a byte copy of the multi-user database, and a cleanup pass rewrites
 * shared state. Both are admin only on the backend, so Settings > System >
 * Data & storage must not offer a member controls that can only answer 403.
 *
 * Runs in the `auth` project (backend restarted with auth required). Serial:
 * the two tests share the admin, member, and snapshot created in beforeAll.
 * A per-file database keeps the single-shot auth setup from colliding with
 * the other auth specs sharing the worker; afterAll restarts to baseline.
 */
test.describe.serial("Data & storage member gating", () => {
  let snapshotName = "";

  test.beforeAll(async ({ backend, browser }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-auth-data-storage.db"),
    });
    const adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(adminContext, backend.baseUrl, GATING_ADMIN);
    await login(adminContext, backend.baseUrl, GATING_ADMIN);
    await createMember(adminContext, backend.baseUrl);
    snapshotName = await createSnapshotAsAdmin(adminContext, backend.baseUrl);
    await adminContext.close();
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("a member keeps the read-only view and no mutating control", async ({
    browser,
    backend,
  }) => {
    const ctx = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(ctx, backend.baseUrl, GATING_MEMBER);
    await expectMemberApiGating(ctx, backend.baseUrl, snapshotName);

    const page = await ctx.newPage();
    await page.goto(DATA_STORAGE_ROUTE);

    // The listing survives the gate: the member still sees the snapshot's
    // name, size and timestamp, which is all the read route now carries.
    const row = page.locator('[data-testid="system-backups-row"]', {
      has: page.locator(`[data-testid="system-backups-name"]`),
    });
    await expect(row.first()).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId("system-backups-name").first()).toHaveText(snapshotName);
    await expect(page.getByTestId("system-backups-admin-only")).toBeVisible();

    // Every backups control that answers 403 is gone, not merely disabled.
    await expect(page.getByTestId("system-backups-create")).toHaveCount(0);
    await expect(page.getByTestId("system-backups-download")).toHaveCount(0);
    await expect(page.getByTestId("system-backups-restore")).toHaveCount(0);
    await expect(page.getByTestId("system-backups-delete")).toHaveCount(0);

    // Storage keeps its controls visible but inert, with the reason attached.
    await expect(page.getByTestId("storage-analyze")).toBeDisabled();
    await expect(page.getByTestId("storage-run-now")).toBeDisabled();

    await ctx.close();
  });

  test("an admin still gets every control on the same page", async ({ browser, backend }) => {
    const ctx = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(ctx, backend.baseUrl, GATING_ADMIN);

    const page = await ctx.newPage();
    await page.goto(DATA_STORAGE_ROUTE);

    await expect(page.getByTestId("system-backups-create")).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId("system-backups-download").first()).toBeVisible();
    await expect(page.getByTestId("system-backups-restore").first()).toBeVisible();
    await expect(page.getByTestId("system-backups-delete").first()).toBeVisible();
    await expect(page.getByTestId("system-backups-admin-only")).toHaveCount(0);
    await expect(page.getByTestId("storage-analyze")).toBeEnabled();
    await expect(page.getByTestId("storage-run-now")).toBeEnabled();

    await ctx.close();
  });
});

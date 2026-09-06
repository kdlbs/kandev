import { expect } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";

const ADMIN = {
  email: "hostnames@demo.dev",
  password: "adminpass123",
  displayName: "Hostname Admin",
};

test.describe.serial("account security hostnames", () => {
  test.beforeEach(async ({ backend }) => {
    const databasePath = path.join(backend.tmpDir, "kandev-hostnames-desktop.db");
    fs.rmSync(databasePath, { force: true });
    fs.rmSync(`${databasePath}-shm`, { force: true });
    fs.rmSync(`${databasePath}-wal`, { force: true });
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: databasePath,
    });
  });
  test.afterEach(async ({ backend }) => {
    const databasePath = path.join(backend.tmpDir, "kandev-hostnames-desktop.db");
    await backend.restart();
    fs.rmSync(databasePath, { force: true });
    fs.rmSync(`${databasePath}-shm`, { force: true });
    fs.rmSync(`${databasePath}-wal`, { force: true });
  });

  test("defaults off and restores the hostname column after re-enabling", async ({
    browser,
    backend,
  }) => {
    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(context, backend.baseUrl, ADMIN);
    const page = await context.newPage();
    await page.goto("/settings/account/security");
    await expect(page.getByTestId("account-sessions-table")).toBeVisible();
    await expect(page.getByText("Hostname", { exact: true })).toHaveCount(0);

    const toggle = page.getByTestId("account-resolve-hostnames-switch");
    await toggle.click();
    await expect(page.getByText("Hostname", { exact: true })).toBeVisible();
    await expect(page.getByTestId("account-session-hostname")).toHaveCount(1);
    await toggle.click();
    await expect(page.getByText("Hostname", { exact: true })).toHaveCount(0);
    await toggle.click();
    await expect(page.getByText("Hostname", { exact: true })).toBeVisible();
    await page.reload();
    await expect(toggle).toHaveAttribute("data-state", "checked");
    await context.close();
  });

  test("shows an error when the hostname preference cannot be saved", async ({
    browser,
    backend,
  }) => {
    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(context, backend.baseUrl, ADMIN);
    const page = await context.newPage();
    await page.goto("/settings/account/security");
    await page.route("**/api/v1/user/settings", async (route) => {
      if (route.request().method() === "PATCH") {
        await route.fulfill({ status: 503, body: "unavailable" });
        return;
      }
      await route.continue();
    });

    await page.getByTestId("account-resolve-hostnames-switch").click();

    await expect(page.getByTestId("account-resolve-hostnames-error")).toHaveText(
      "Could not update hostname resolution.",
    );
    await context.close();
  });

  test("refetches sessions after a new login while the first response is pending", async ({
    browser,
    backend,
  }) => {
    const firstContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(firstContext, backend.baseUrl, ADMIN);
    const initialResponse = await firstContext.request.get(
      `${backend.baseUrl}/api/v1/auth/sessions`,
    );
    expect(initialResponse.ok(), await initialResponse.text()).toBeTruthy();
    const initialSessions = await initialResponse.json();
    const page = await firstContext.newPage();
    let releaseInitialResponse: (() => void) | undefined;
    const initialResponseHeld = new Promise<void>((resolve) => {
      releaseInitialResponse = resolve;
    });
    let sessionRequests = 0;
    await page.route("**/api/v1/auth/sessions", async (route) => {
      sessionRequests += 1;
      if (sessionRequests === 1) {
        await initialResponseHeld;
        await route.fulfill({ json: initialSessions });
        return;
      }
      await route.continue();
    });
    await page.goto("/settings/account/security");
    await expect.poll(() => sessionRequests).toBe(1);

    const secondContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(secondContext, backend.baseUrl, ADMIN);
    const secondPage = await secondContext.newPage();
    await secondPage.goto("/settings/account/security");
    await page.bringToFront();
    await page.evaluate(() => window.dispatchEvent(new FocusEvent("focus")));
    releaseInitialResponse?.();

    await expect(page.getByTestId("account-sessions-row")).toHaveCount(2);
    await firstContext.close();
    await secondContext.close();
  });
});

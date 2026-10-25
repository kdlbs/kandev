import { devices, expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { setupAdmin } from "../../helpers/auth";

const ADMIN = {
  email: "hostnames-mobile@demo.dev",
  password: "adminpass123",
  displayName: "Hostname Mobile",
};

test.describe.serial("account security hostnames mobile", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-hostnames-mobile.db"),
    });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("enables and hides hostnames without page overflow", async ({ browser, backend }) => {
    const context = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    await setupAdmin(context, backend.baseUrl, ADMIN);
    const page = await context.newPage();
    await page.goto("/settings/account/security");
    const toggle = page.getByTestId("account-resolve-hostnames-switch");
    await expect(toggle).toBeVisible();
    await toggle.tap();
    await expect(page.getByText("Hostname", { exact: true })).toBeVisible();
    await expect(page.getByTestId("account-session-hostname")).toHaveCount(1);
    await expect(
      page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).resolves.toBe(true);
    await toggle.tap();
    await expect(page.getByText("Hostname", { exact: true })).toHaveCount(0);
    await context.close();
  });
});

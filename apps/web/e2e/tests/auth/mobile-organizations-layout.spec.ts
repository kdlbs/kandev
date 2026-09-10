import { devices, expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";

const ADMIN = {
  email: "operator@e2e.dev",
  password: "operatorpass123",
  displayName: "Olivia Operator",
};

test.describe.serial("organization management layout (mobile)", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_FEATURES_MULTI_TENANCY: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-mobile-organizations.db"),
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("keeps organization identity and actions usable on a phone", async ({
    browser,
    backend,
  }) => {
    const context = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });

    try {
      await setupAdmin(context, backend.baseUrl, ADMIN);
      await login(context, backend.baseUrl, ADMIN);

      const created = await context.request.post(`${backend.baseUrl}/api/v1/instance/orgs`, {
        data: { name: "Globex Industries" },
      });
      expect(created.status(), await created.text()).toBe(200);

      const page = await context.newPage();
      expect((await page.viewportSize())?.width).toBe(393);
      await page.goto("/settings/system/organizations");

      const row = page.getByTestId("organization-row").filter({ hasText: "Globex Industries" });
      await expect(row).toBeVisible({ timeout: 15_000 });

      const name = row.getByText("Globex Industries", { exact: true });
      expect(
        await name.evaluate((element) => element.scrollWidth <= element.clientWidth),
        "the organization name must render in full instead of collapsing to an ellipsis",
      ).toBe(true);

      for (const accessibleName of [/add administrator/i, /^suspend$/i, /delete organization/i]) {
        const action = row.getByRole("button", { name: accessibleName });
        await expect(action).toBeVisible();
        const box = await action.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.height).toBeGreaterThanOrEqual(44);
      }

      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);

      const suspended = page.waitForResponse(
        (response) =>
          response.request().method() === "PATCH" &&
          /\/api\/v1\/instance\/orgs\/[^/]+$/.test(new URL(response.url()).pathname),
      );
      await row.getByRole("button", { name: /^suspend$/i }).tap();
      expect((await suspended).status()).toBe(200);
      await expect(row.getByText("Suspended", { exact: true })).toBeVisible();
    } finally {
      await context.close();
    }
  });
});

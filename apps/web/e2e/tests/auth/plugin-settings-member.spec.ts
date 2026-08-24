import { expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

const ADMIN = { email: "plugin-admin@demo.dev", password: "adminpass123", displayName: "Ada" };
const MEMBER = { email: "plugin-member@demo.dev", password: "memberpass123", displayName: "Sam" };

test.describe.serial("member plugin settings", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-auth-plugin-member.db"),
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("shows plugin and marketplace metadata without administrator controls", async ({
    browser,
    backend,
  }) => {
    const adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(adminContext, backend.baseUrl, ADMIN);
    await login(adminContext, backend.baseUrl, ADMIN);

    const adminPage = await adminContext.newPage();
    await installFixturePlugin(adminPage);
    const createMember = await adminContext.request.post(`${backend.baseUrl}/api/v1/users`, {
      data: {
        email: MEMBER.email,
        password: MEMBER.password,
        display_name: MEMBER.displayName,
        role: "member",
      },
    });
    expect(createMember.status(), await createMember.text()).toBe(201);

    const memberContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(memberContext, backend.baseUrl, MEMBER);
    const page = await memberContext.newPage();
    await page.goto("/settings/plugins");

    const settingsPanel = page.locator('[data-testid="settings-scroll-container"]:visible');
    await expect(settingsPanel).toBeVisible();

    const row = settingsPanel.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row.getByTestId(`plugin-row-link-${PLUGIN_ID}`)).toBeVisible();
    await expect(settingsPanel.getByTestId("install-plugin-trigger")).toHaveCount(0);
    await expect(settingsPanel.getByTestId("plugins-sync-button")).toHaveCount(0);
    await expect(settingsPanel.getByTestId("plugins-check-updates-button")).toHaveCount(0);
    await expect(settingsPanel.getByTestId("plugins-auto-update-default")).toHaveCount(0);
    await expect(row.getByRole("button", { name: /Enable|Disable|Uninstall|Update/ })).toHaveCount(
      0,
    );
    await expect(row.getByTestId(`plugin-auto-update-${PLUGIN_ID}`)).toHaveCount(0);

    await row.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();
    await expect(settingsPanel.getByTestId("plugin-manifest-card")).toBeVisible();
    await expect(settingsPanel.getByTestId("plugin-settings-card")).toHaveCount(0);
    await expect(settingsPanel.getByRole("button", { name: "Uninstall" })).toHaveCount(0);

    await page.goto("/settings/plugins");
    await settingsPanel.getByTestId("plugins-tab-browse").click();
    await expect(settingsPanel.getByTestId("marketplace-search")).toBeVisible();
    await expect(settingsPanel.getByTestId("marketplace-manage-sources")).toHaveCount(0);
    await expect(settingsPanel.getByRole("button", { name: "Refresh" })).toHaveCount(0);

    await memberContext.close();
    await adminContext.close();
  });
});

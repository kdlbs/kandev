import { expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { waitForHttp } from "../../helpers/causal-waits";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

const ADMIN = { email: "plugin-admin@demo.dev", password: "adminpass123", displayName: "Ada" };
const MEMBER = { email: "plugin-member@demo.dev", password: "memberpass123", displayName: "Sam" };
const PLUGIN_SHORTCUT_ID = `plugin:${PLUGIN_ID}:open-demo`;

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
    const baselineSettingsResponse = await memberContext.request.get(
      `${backend.baseUrl}/api/v1/user/settings`,
    );
    expect(baselineSettingsResponse.status(), await baselineSettingsResponse.text()).toBe(200);
    const baselineSettings = (await baselineSettingsResponse.json()).settings as {
      keyboard_shortcuts?: Record<string, unknown>;
    };

    try {
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
      await expect(
        row.getByRole("button", { name: /Enable|Disable|Uninstall|Update/ }),
      ).toHaveCount(0);
      await expect(row.getByTestId(`plugin-auto-update-${PLUGIN_ID}`)).toHaveCount(0);

      // Plugin-owned shortcut rows are not part of the global Kandev shortcut page.
      await page.goto("/settings/preferences/keyboard-shortcuts");
      await expect(page.getByTestId("shortcut-recorder-SEARCH")).toBeVisible();
      await expect(page.getByTestId(`shortcut-recorder-${PLUGIN_SHORTCUT_ID}`)).toHaveCount(0);

      await page.goto("/settings/plugins");
      await row.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();
      await expect(settingsPanel.getByTestId("plugin-manifest-card")).toBeVisible();
      await expect(settingsPanel.getByTestId("plugin-settings-card")).toHaveCount(0);
      await expect(settingsPanel.getByRole("button", { name: "Uninstall" })).toHaveCount(0);

      // Members can edit personal plugin shortcuts even though operator controls stay hidden.
      const shortcutCard = settingsPanel.getByTestId("plugin-shortcuts-card");
      await expect(shortcutCard).toBeVisible();
      const shortcutRecorder = shortcutCard.getByTestId(`shortcut-recorder-${PLUGIN_SHORTCUT_ID}`);
      await expect(shortcutRecorder).toBeVisible();
      await shortcutRecorder.click();
      await page.keyboard.press("Control+Alt+p");
      await expect(shortcutRecorder).toHaveText("Ctrl+Alt+P");
      const settingsSaved = waitForHttp(page, "PATCH", /^\/api\/v1\/user\/settings$/, {
        predicate: (response) => response.ok(),
      });
      await page
        .getByTestId("settings-floating-save")
        .getByRole("button", {
          name: "Save changes",
        })
        .click();
      await settingsSaved;

      await page.reload();
      await expect(settingsPanel.getByTestId(`shortcut-recorder-${PLUGIN_SHORTCUT_ID}`)).toHaveText(
        "Ctrl+Alt+P",
      );

      await page.goto("/settings/plugins");
      await settingsPanel.getByTestId("plugins-tab-browse").click();
      await expect(settingsPanel.getByTestId("marketplace-search")).toBeVisible();
      await expect(settingsPanel.getByTestId("marketplace-manage-sources")).toHaveCount(0);
      await expect(settingsPanel.getByRole("button", { name: "Refresh" })).toHaveCount(0);
    } finally {
      const restore = await memberContext.request.patch(`${backend.baseUrl}/api/v1/user/settings`, {
        data: { keyboard_shortcuts: baselineSettings.keyboard_shortcuts ?? {} },
      });
      expect(restore.status(), await restore.text()).toBe(200);
      await memberContext.close();
      await adminContext.close();
    }
  });
});

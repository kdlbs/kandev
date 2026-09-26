import { expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

const ADMIN = {
  email: "mobile-plugin-admin@demo.dev",
  password: "adminpass123",
  displayName: "Ada",
};
const MEMBER = {
  email: "mobile-plugin-member@demo.dev",
  password: "memberpass123",
  displayName: "Sam",
};

test.describe.serial("member plugin settings on mobile", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-auth-mobile-plugin-member.db"),
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("shows declared attribution and trust state without admin controls", async ({
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
    try {
      await page.goto("/settings/plugins");
      const settingsPanel = page.locator('[data-testid="settings-scroll-container"]:visible');
      const row = settingsPanel.getByTestId(`plugin-row-${PLUGIN_ID}`);
      await expect(row).toBeVisible({ timeout: 15_000 });
      await row.getByTestId(`plugin-row-link-${PLUGIN_ID}`).tap();

      const detail = settingsPanel.getByTestId(`plugin-detail-${PLUGIN_ID}`);
      await expect(detail).toBeVisible();
      const publisherIdentity = detail.getByTestId("plugin-publisher-identity");
      await expect(publisherIdentity).toBeVisible();
      await expect(
        publisherIdentity.getByText("Unverified publisher", { exact: true }),
      ).toBeVisible();
      await expect(publisherIdentity.getByText("Declared author:", { exact: true })).toBeVisible();
      await expect(publisherIdentity.getByText("kandev", { exact: true })).toBeVisible();
      await expect(detail.getByTestId("plugin-verify-publisher")).toHaveCount(0);
    } finally {
      await memberContext.close();
      await adminContext.close();
    }
  });
});

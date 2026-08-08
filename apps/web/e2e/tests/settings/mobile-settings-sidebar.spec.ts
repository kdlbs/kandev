import { test, expect } from "../../fixtures/test-base";

test.describe("Mobile settings navigation", () => {
  test("keeps the desktop settings sidebar hidden without leaking the old active text", async ({
    testPage,
  }) => {
    await testPage.goto("/settings");

    await expect(testPage.getByRole("link", { name: /Appearance/ })).toBeVisible();
    await expect(testPage.getByRole("link", { name: /Terminal/ })).toBeVisible();
    await expect(testPage.getByText("[active]")).toHaveCount(0);
    await expect(testPage.getByTestId("app-sidebar")).toBeHidden();

    const takeover = testPage.getByTestId("app-sidebar-settings-mode");

    await expect(takeover).toBeHidden();

    await testPage.getByTestId("settings-mobile-menu-button").click();
    await expect(
      testPage.getByTestId("settings-mobile-menu").locator('a[href="/settings/integrations"]'),
    ).toBeVisible();
  });

  test("shows the workspace breadcrumb and orders secrets after automations", async ({
    testPage,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);

    await expect(testPage.getByRole("navigation", { name: "Breadcrumb" })).toContainText(
      "E2E Workspace",
    );

    await testPage.getByTestId("settings-mobile-menu-button").click();
    const menu = testPage.getByTestId("settings-mobile-menu");
    const hrefs = await menu
      .locator("a")
      .evaluateAll((links) =>
        links
          .map((link) => link.getAttribute("href"))
          .filter((href): href is string => Boolean(href)),
      );

    const automationsIndex = hrefs.indexOf(
      `/settings/workspace/${seedData.workspaceId}/automations`,
    );
    const secretsIndex = hrefs.indexOf(`/settings/workspace/${seedData.workspaceId}/secrets`);
    expect(automationsIndex).toBeGreaterThanOrEqual(0);
    expect(secretsIndex).toBeGreaterThan(automationsIndex);
  });
});

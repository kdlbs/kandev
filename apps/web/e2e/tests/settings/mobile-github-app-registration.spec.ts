import { test, expect } from "../../fixtures/test-base";
import { GitHubAuthSettingsPage } from "../../pages/github-auth-settings-page";
import {
  assertLocatorWithinViewportX,
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import type { Locator } from "@playwright/test";

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

async function expectDrawerOwnsScrolling(drawer: Locator) {
  const state = await drawer.evaluate((node) => {
    const scrollOwner = node.querySelector(".overflow-y-auto") as HTMLElement | null;
    if (!scrollOwner) return null;
    scrollOwner.scrollTop = scrollOwner.scrollHeight;
    return {
      drawerOverflow: getComputedStyle(node).overflowY,
      scrollOwnerOverflow: getComputedStyle(scrollOwner).overflowY,
      internalScroll: scrollOwner.scrollHeight > scrollOwner.clientHeight,
      internalScrollTop: scrollOwner.scrollTop,
      documentScrollTop: window.scrollY,
    };
  });
  expect(state).not.toBeNull();
  expect(state!.drawerOverflow).toBe("hidden");
  expect(state!.scrollOwnerOverflow).toBe("auto");
  expect(state!.internalScroll).toBe(true);
  expect(state!.internalScrollTop).toBeGreaterThan(0);
  expect(state!.documentScrollTop).toBe(0);
}

test.describe("Mobile workspace GitHub App onboarding", () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.mockGitHubReset();
  });

  test("keeps App choice and existing-App instructions touch-safe in one scrolling drawer", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const otherWorkspace = await apiClient.createWorkspace("Mobile Shared Workspace");
    await apiClient.mockGitHubSetAppRegistration({
      id: "registration-mobile-shared",
      display_name: "Company automation with a long name",
      app_id: 601,
    });
    const connection = {
      source: "github_app_installation" as const,
      status: "active" as const,
      app_registration_id: "registration-mobile-shared",
      installation_id: 61,
      installation_account_login: "acme-engineering-with-a-long-name",
      installation_account_type: "Organization",
      capabilities: { repository_read: true },
    };
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, connection);
    await apiClient.mockGitHubSetWorkspaceConnection(otherWorkspace.id, {
      ...connection,
      installation_id: 62,
    });

    const settings = new GitHubAuthSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    await expect(
      settings.automation().getByText("acme-engineering-with-a-long-name", { exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    const drawer = await settings.openConnection();
    await expect(drawer).toBeVisible();
    await assertLocatorWithinViewportX(drawer, "GitHub connection drawer");

    for (const method of ["pat", "cli", "app"]) {
      await expectTouchTarget(drawer.locator(`label[for="github-method-${method}"]`));
    }
    await settings.chooseMethod("GitHub App");
    await expect(drawer.getByText(/Used by 2 workspaces/)).toBeVisible();
    await expectTouchTarget(drawer.getByTestId("github-app-install-button"));
    await expectTouchTarget(drawer.getByRole("button", { name: "Add existing App" }));

    await drawer.getByRole("button", { name: "Add existing App" }).click();
    await drawer.getByLabel("Public Kandev URL").fill("https://1.1.1.1");
    await drawer.getByRole("button", { name: "Generate setup instructions" }).click();
    await expect(drawer.getByText("Configure the existing App on GitHub")).toBeVisible();
    await expect(drawer.getByText("User authorization callback URL")).toBeVisible();
    await expect(drawer.getByText("Webhook URL", { exact: true })).toBeVisible();
    await expectTouchTarget(drawer.getByRole("button", { name: "Copy Webhook URL" }));
    await expectTouchTarget(drawer.getByRole("button", { name: "Verify and import App" }));

    await expectDrawerOwnsScrolling(drawer);
    await assertNoDescendantOverflowsRight(drawer, "mobile GitHub App import drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile GitHub App import");
    await testPage.screenshot({
      path: testInfo.outputPath("workspace-github-app-import-mobile.png"),
      fullPage: true,
    });
  });

  test("handles installation and personal identity callback states on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubSetAppRegistration({
      id: "registration-mobile",
      display_name: "Mobile App",
      app_id: 701,
    });
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "github_app_installation",
      status: "active",
      app_registration_id: "registration-mobile",
      installation_id: 71,
      installation_account_login: "mobile-org",
      installation_account_type: "Organization",
      capabilities: { repository_read: true },
    });

    const settings = new GitHubAuthSettingsPage(testPage);
    await settings.goto(
      seedData.workspaceId,
      `?github_result=app_connected&workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
    );
    await expect(testPage.getByTestId("github-callback-notice")).toContainText(
      "GitHub App connected",
    );
    const personal = settings.personalIdentity();
    const connect = personal.getByRole("button", { name: "Connect identity" });
    await expectTouchTarget(connect);

    let personalStartBody: Record<string, unknown> | undefined;
    await testPage.route("**/api/v1/github/personal-connection/start", async (route) => {
      personalStartBody = route.request().postDataJSON() as Record<string, unknown>;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ url: "https://github.com/login/oauth/authorize?mock=mobile" }),
      });
    });
    await testPage.route("https://github.com/login/oauth/authorize?mock=mobile", (route) =>
      route.fulfill({
        status: 200,
        contentType: "text/html",
        body: "<main><h1>Authorize personal identity</h1></main>",
      }),
    );
    await connect.click();
    await expect(
      testPage.getByRole("heading", { name: "Authorize personal identity" }),
    ).toBeVisible();
    expect(personalStartBody).toEqual({ workspace_id: seedData.workspaceId });

    await apiClient.mockGitHubSetPersonalConnection(seedData.workspaceId, {
      login: "mobile-personal-user",
      status: "active",
      github_user_id: 72,
      access_expires_at: "2030-01-01T00:00:00Z",
    });
    await settings.goto(
      seedData.workspaceId,
      `?github_result=personal_connected&workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
    );
    await expect(testPage.getByTestId("github-callback-notice")).toContainText(
      "GitHub identity connected",
    );
    await expect(
      settings.personalIdentity().getByText("mobile-personal-user", { exact: true }),
    ).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile GitHub callback state");
  });
});

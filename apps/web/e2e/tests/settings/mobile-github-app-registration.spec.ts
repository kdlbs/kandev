import { test, expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import {
  assertLocatorWithinViewportX,
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";

const SYSTEM_GITHUB_APP_PATH = "/settings/system/github-app";

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

async function expectSettingsOwnsScrolling(page: Page) {
  const scrollContainer = page.getByTestId("settings-scroll-container");
  const state = await scrollContainer.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
    return {
      overflowY: getComputedStyle(node).overflowY,
      internalScroll: node.scrollHeight > node.clientHeight,
      internalScrollTop: node.scrollTop,
      documentScrollTop: window.scrollY,
    };
  });
  expect(["auto", "scroll"]).toContain(state.overflowY);
  expect(state.internalScroll).toBe(true);
  expect(state.internalScrollTop).toBeGreaterThan(0);
  expect(state.documentScrollTop).toBe(0);
}

test.describe("Mobile deployment GitHub App onboarding", () => {
  test("validates setup, reviews permissions, and safely cancels the GitHub handoff", async ({
    testPage,
  }, testInfo) => {
    await testPage.goto(SYSTEM_GITHUB_APP_PATH);
    const settings = testPage.getByTestId("github-app-settings");
    await expect(settings).toBeVisible();
    await expect(testPage.getByTestId("github-app-status")).toHaveAttribute(
      "data-state",
      "unconfigured",
    );

    const create = testPage.getByTestId("github-app-create-button");
    const review = testPage.getByTestId("github-app-permissions-button");
    await expectTouchTarget(create);
    await expectTouchTarget(review);
    await expectTouchTarget(
      testPage.getByText("Organization", { exact: true }).locator("xpath=ancestor::label"),
    );
    await assertLocatorWithinViewportX(create, "mobile create action");
    await assertLocatorWithinViewportX(review, "mobile permission action");

    await create.click();
    await expect(testPage.getByText("Enter the GitHub organization login.")).toBeVisible();
    await expect(testPage.getByText("Enter a public HTTPS origin.")).toBeVisible();

    await review.click();
    const permissions = testPage.getByRole("dialog", { name: "GitHub App permissions" });
    const permissionList = testPage.getByTestId("github-app-permissions-list");
    await assertLocatorWithinViewportX(permissions, "mobile permissions dialog");
    const permissionScroll = await permissionList.evaluate((node) => ({
      overflowY: getComputedStyle(node).overflowY,
      scrollable: node.scrollHeight > node.clientHeight,
    }));
    expect(permissionScroll.overflowY).toBe("auto");
    expect(permissionScroll.scrollable).toBe(true);
    await expect(permissions.getByText("Workflows", { exact: true })).toBeVisible();
    await permissions.getByRole("button", { name: "Close" }).click();

    await testPage.getByLabel("Organization login").fill("acme");
    await testPage.getByLabel("Public Kandev URL").fill("https://1.1.1.1");
    await create.click();
    const handoff = testPage.getByTestId("github-app-manifest-confirm");
    await expect(handoff).toBeVisible();
    await assertLocatorWithinViewportX(handoff, "mobile manifest handoff");
    await expectTouchTarget(handoff.getByRole("button", { name: "Stay in Kandev" }));
    await expectTouchTarget(handoff.getByTestId("github-app-manifest-continue"));

    await testPage.screenshot({
      path: testInfo.outputPath("github-app-manifest-handoff-mobile.png"),
      fullPage: true,
    });

    await handoff.getByRole("button", { name: "Stay in Kandev" }).click();
    await expect(handoff).not.toBeVisible();
    await expect(testPage.getByLabel("Organization login")).toHaveValue("acme");
    await expectSettingsOwnsScrolling(testPage);
    await assertNoDescendantOverflowsRight(settings, "mobile GitHub App settings");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile setup");
  });

  test("hot-enables the App and reaches workspace installation without a restart", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    await apiClient.mockGitHubSetDeploymentAppRegistration({
      source: "managed",
      state: "ready",
      ready: true,
      app_id: 4242,
      slug: "kandev-acme",
      owner_login: "acme",
      owner_type: "Organization",
      public_base_url: "https://kandev.acme.test",
      webhook_status: "unverified",
    });
    await testPage.goto(`${SYSTEM_GITHUB_APP_PATH}?github_app_result=connected`);

    await expect(testPage.getByTestId("github-app-callback-result")).toContainText(
      "GitHub App created",
    );
    await expect(testPage.getByTestId("github-app-status")).toHaveAttribute("data-state", "ready");
    const workspaceHandoff = testPage.getByTestId("github-app-workspace-handoff");
    await expectTouchTarget(workspaceHandoff);
    await assertLocatorWithinViewportX(workspaceHandoff, "mobile workspace handoff");

    await workspaceHandoff.click();
    await expect(testPage).toHaveURL(/\/settings\/integrations\/github$/);
    await testPage
      .getByTestId("github-workspace-automation")
      .getByRole("button", {
        name: "Connect GitHub",
      })
      .click();
    const dialog = testPage.getByRole("dialog", { name: "Connect GitHub" });
    await dialog.getByRole("combobox", { name: "Connection method" }).click();
    await testPage.getByRole("option", { name: "GitHub App", exact: true }).click();

    const install = testPage.getByTestId("github-app-install-button");
    await expectTouchTarget(install);
    await assertLocatorWithinViewportX(dialog, "mobile workspace connection dialog");
    await assertLocatorWithinViewportX(install, "mobile install action");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile workspace install");

    await testPage.route("**/api/v1/github/app/install/start", async (route) => {
      expect(route.request().postDataJSON()).toEqual({ workspace_id: seedData.workspaceId });
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ url: "https://github.com/apps/kandev-acme/installations/new" }),
      });
    });
    await testPage.route("https://github.com/apps/kandev-acme/installations/new", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "text/html",
        body: "<main><h1>Mock GitHub installation</h1></main>",
      });
    });

    await testPage.screenshot({
      path: testInfo.outputPath("github-app-workspace-install-mobile.png"),
      fullPage: true,
    });
    await install.click();
    await expect(testPage.getByRole("heading", { name: "Mock GitHub installation" })).toBeVisible();
  });

  test("keeps environment setup immutable and bound managed credentials intact", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockGitHubSetDeploymentAppRegistration({
      source: "environment",
      state: "ready",
      ready: true,
      app_id: 8080,
      slug: "operator-managed-app",
      public_base_url: "https://kandev.example.com",
      webhook_status: "verified",
    });
    await testPage.goto(SYSTEM_GITHUB_APP_PATH);
    await expect(testPage.getByTestId("github-app-environment-status")).toBeVisible();
    await expect(testPage.getByTestId("github-app-remove-button")).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile environment status");

    await apiClient.mockGitHubSetDeploymentAppRegistration({
      source: "managed",
      state: "ready",
      ready: true,
      app_id: 4242,
      slug: "kandev-acme",
      owner_login: "acme",
      owner_type: "Organization",
      public_base_url: "https://kandev.acme.test",
      webhook_status: "verified",
    });
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "github_app_installation",
      status: "active",
      installation_id: 42,
      installation_account_login: "acme",
      installation_account_type: "Organization",
    });
    await testPage.reload();
    const remove = testPage.getByTestId("github-app-remove-button");
    await expectTouchTarget(remove);
    await remove.click();
    const confirmation = testPage.getByTestId("github-app-remove-confirmation");
    await expectTouchTarget(confirmation);
    await confirmation.fill("DELETE");
    const confirm = testPage.getByTestId("github-app-remove-confirm");
    await expectTouchTarget(confirm);
    await confirm.click();

    await expect(testPage.getByText("deployment GitHub App is used by a workspace")).toBeVisible();
    await expect(testPage.getByText("kandev-acme", { exact: true })).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile blocked removal");
  });
});

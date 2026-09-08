import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { SentrySettingsPage } from "../../pages/sentry-settings-page";

function guardAgainstNativeDialogs(testPage: Page) {
  let seen = false;
  testPage.on("dialog", async (dialog) => {
    seen = true;
    await dialog.dismiss();
  });
  return () => seen;
}

async function expectTouchSized(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

async function expectNoHorizontalOverflow(testPage: Page) {
  await expect
    .poll(() => testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
    .toBe(true);
}

test.describe("integration configuration removal confirmations on mobile", () => {
  test("Azure DevOps uses inline touch confirmation", async ({ testPage, apiClient, seedData }) => {
    const sawNativeDialog = guardAgainstNativeDialogs(testPage);
    await apiClient.setAzureDevOpsConfig(seedData.workspaceId, {
      organizationUrl: "https://dev.azure.com/acme",
      pat: "azure-mobile-pat",
    });
    await testPage.goto(
      `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/azure-devops`,
    );

    const removeButton = testPage.getByTestId("azure-devops-delete-button");
    await removeButton.tap();
    const inline = testPage.getByTestId("azure-devops-remove-inline-confirmation");
    await expect(inline).toBeVisible();
    await expectTouchSized(inline.getByTestId("azure-devops-remove-confirm"));
    await inline.getByRole("button", { name: "Cancel" }).tap();
    await expect(removeButton).toBeVisible();
    await removeButton.tap();
    await inline.getByTestId("azure-devops-remove-confirm").tap();
    await expect(removeButton).toHaveCount(0);
    expect(sawNativeDialog()).toBe(false);
    await expectNoHorizontalOverflow(testPage);
  });

  test("Jira uses inline touch confirmation", async ({ testPage, apiClient, seedData }) => {
    const sawNativeDialog = guardAgainstNativeDialogs(testPage);
    await apiClient.setJiraConfig({
      workspaceId: seedData.workspaceId,
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "jira-mobile-token",
    });
    await testPage.goto(
      `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/jira`,
    );

    const removeButton = testPage.getByTestId("jira-delete-button");
    await removeButton.tap();
    const inline = testPage.getByTestId("jira-remove-inline-confirmation");
    await expect(inline).toBeVisible();
    await expectTouchSized(inline.getByTestId("jira-remove-confirm"));
    await inline.getByRole("button", { name: "Cancel" }).tap();
    await expect(removeButton).toBeVisible();
    await removeButton.tap();
    await inline.getByTestId("jira-remove-confirm").tap();
    await expect(removeButton).toHaveCount(0);
    expect(sawNativeDialog()).toBe(false);
    await expectNoHorizontalOverflow(testPage);
  });

  test("Linear uses inline touch confirmation", async ({ testPage, apiClient, seedData }) => {
    const sawNativeDialog = guardAgainstNativeDialogs(testPage);
    await apiClient.setLinearConfig({
      workspaceId: seedData.workspaceId,
      secret: "lin_api_mobile",
    });
    await testPage.goto(
      `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/linear`,
    );

    const removeButton = testPage.getByTestId("linear-delete-button");
    await removeButton.tap();
    const inline = testPage.getByTestId("linear-remove-inline-confirmation");
    await expect(inline).toBeVisible();
    await expectTouchSized(inline.getByTestId("linear-remove-confirm"));
    await inline.getByRole("button", { name: "Cancel" }).tap();
    await expect(removeButton).toBeVisible();
    await removeButton.tap();
    await inline.getByTestId("linear-remove-confirm").tap();
    await expect(removeButton).toHaveCount(0);
    expect(sawNativeDialog()).toBe(false);
    await expectNoHorizontalOverflow(testPage);
  });

  test("Sentry uses an inline confirmation identified by instance", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sawNativeDialog = guardAgainstNativeDialogs(testPage);
    await apiClient.mockSentryReset();
    await apiClient.createSentryInstance({
      workspaceId: seedData.workspaceId,
      name: "Mobile Sentry",
      secret: "sntrys_mobile",
    });
    const settings = new SentrySettingsPage(testPage);
    await settings.goto(seedData.workspaceId);

    const card = settings.cardByName("Mobile Sentry");
    const removeButton = card.getByTestId("sentry-instance-delete-button");
    await removeButton.tap();
    const inline = card.getByTestId("sentry-remove-inline-confirmation");
    await expect(inline).toBeVisible();
    await expect(inline).toContainText("Mobile Sentry");
    await expectTouchSized(inline.getByTestId("sentry-remove-confirm"));
    await inline.getByRole("button", { name: "Cancel" }).tap();
    await expect(removeButton).toBeVisible();
    await removeButton.tap();
    await inline.getByTestId("sentry-remove-confirm").tap();
    await expect(settings.cardByName("Mobile Sentry")).toHaveCount(0);
    expect(sawNativeDialog()).toBe(false);
    await expectNoHorizontalOverflow(testPage);
  });
});

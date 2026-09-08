import type { Page } from "@playwright/test";
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

test.describe("integration configuration removal confirmations", () => {
  test("Azure DevOps removal stays local and confirms once", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sawNativeDialog = guardAgainstNativeDialogs(testPage);
    await apiClient.setAzureDevOpsConfig(seedData.workspaceId, {
      organizationUrl: "https://dev.azure.com/acme",
      pat: "azure-test-pat",
    });
    await testPage.goto(
      `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/azure-devops`,
    );

    const removeButton = testPage.getByTestId("azure-devops-delete-button");
    await expect(removeButton).toBeVisible();
    await removeButton.click();
    const popover = testPage.getByTestId("azure-devops-remove-confirm-popover");
    await expect(popover).toContainText("Azure DevOps");
    await popover.getByRole("button", { name: "Cancel" }).click();
    await expect(popover).toHaveCount(0);
    await expect(removeButton).toBeVisible();

    await removeButton.click();
    await testPage.getByTestId("azure-devops-remove-confirm").click();
    await expect(removeButton).toHaveCount(0);
    await expect(testPage.getByTestId("azure-devops-organization")).toHaveValue("");
    expect(sawNativeDialog()).toBe(false);
  });

  test("Jira removal stays local and confirms once", async ({ testPage, apiClient, seedData }) => {
    const sawNativeDialog = guardAgainstNativeDialogs(testPage);
    await apiClient.setJiraConfig({
      workspaceId: seedData.workspaceId,
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "jira-test-token",
    });
    await testPage.goto(
      `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/jira`,
    );

    const removeButton = testPage.getByTestId("jira-delete-button");
    await expect(removeButton).toBeVisible();
    await removeButton.click();
    const popover = testPage.getByTestId("jira-remove-confirm-popover");
    await expect(popover).toContainText("Jira");
    await popover.getByRole("button", { name: "Cancel" }).click();
    await expect(popover).toHaveCount(0);
    await expect(removeButton).toBeVisible();

    await removeButton.click();
    await testPage.getByTestId("jira-remove-confirm").click();
    await expect(removeButton).toHaveCount(0);
    await expect(testPage.getByTestId("jira-site-input")).toHaveValue("");
    expect(sawNativeDialog()).toBe(false);
  });

  test("Linear removal stays local and confirms once", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sawNativeDialog = guardAgainstNativeDialogs(testPage);
    await apiClient.setLinearConfig({
      workspaceId: seedData.workspaceId,
      secret: "lin_api_test",
    });
    await testPage.goto(
      `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/linear`,
    );

    const removeButton = testPage.getByTestId("linear-delete-button");
    await expect(removeButton).toBeVisible();
    await removeButton.click();
    const popover = testPage.getByTestId("linear-remove-confirm-popover");
    await expect(popover).toContainText("Linear");
    await popover.getByRole("button", { name: "Cancel" }).click();
    await expect(popover).toHaveCount(0);
    await expect(removeButton).toBeVisible();

    await removeButton.click();
    await testPage.getByTestId("linear-remove-confirm").click();
    await expect(removeButton).toHaveCount(0);
    await expect(testPage.getByTestId("linear-secret-input")).toHaveValue("");
    expect(sawNativeDialog()).toBe(false);
  });

  test("Sentry removal identifies the instance and confirms once", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sawNativeDialog = guardAgainstNativeDialogs(testPage);
    await apiClient.mockSentryReset();
    await apiClient.createSentryInstance({
      workspaceId: seedData.workspaceId,
      name: "Production Sentry",
      secret: "sntrys_test",
    });
    const settings = new SentrySettingsPage(testPage);
    await settings.goto(seedData.workspaceId);

    const card = settings.cardByName("Production Sentry");
    const removeButton = card.getByTestId("sentry-instance-delete-button");
    await removeButton.click();
    const popover = testPage.getByTestId("sentry-remove-confirm-popover");
    await expect(popover).toContainText("Production Sentry");
    await popover.getByRole("button", { name: "Cancel" }).click();
    await expect(popover).toHaveCount(0);
    await expect(removeButton).toBeVisible();

    await removeButton.click();
    await testPage.getByTestId("sentry-remove-confirm").click();
    await expect(settings.cardByName("Production Sentry")).toHaveCount(0);
    expect(sawNativeDialog()).toBe(false);
  });
});

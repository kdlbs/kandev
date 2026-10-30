import type { BrowserContext, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

// Covers AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.1, .2, .4, .5, .6, .7 and 003.1
// against the backend's mock Microsoft authority (KANDEV_MOCK_AZURE_DEVOPS).

const ORGANIZATION_URL = "https://dev.azure.com/acme";
const MOCK_AUTHORIZE = "**/api/v1/azure-devops/mock/oauth/authorize*";
const MOCK_CODE = "mock-authorization-code";
const COMPLETE_PATH = /\/api\/v1\/azure-devops\/oauth\/complete$/;
const MOCK_STATE = {
  authenticated: true,
  user: { ok: true, id: "user-1", displayName: "Ada Reviewer" },
  projects: [{ id: "project-1", name: "Platform", url: "https://dev.azure.com/acme/Platform" }],
};

function settingsPath(workspaceId: string): string {
  return `/settings/workspaces/${encodeURIComponent(workspaceId)}/integrations/azure-devops`;
}

async function selectMicrosoftAccount(page: Page) {
  await page.getByTestId("azure-devops-auth-method").click();
  await page.getByRole("option", { name: "Microsoft account", exact: true }).click();
  await expect(page.getByTestId("azure-devops-oauth")).toBeVisible();
  await expect(page.getByTestId("azure-devops-pat")).toHaveCount(0);
}

/** Holds the mock authorize page so the popup never posts back, and returns
 *  the callback address Microsoft would have redirected to. */
async function holdAuthorizeRedirect(context: BrowserContext): Promise<() => Promise<string>> {
  let resolveState: (state: string) => void = () => undefined;
  const statePromise = new Promise<string>((resolve) => {
    resolveState = resolve;
  });
  await context.route(MOCK_AUTHORIZE, async (route) => {
    resolveState(new URL(route.request().url()).searchParams.get("state") ?? "");
    await route.fulfill({ contentType: "text/html", body: "<!doctype html><title>hold</title>" });
  });
  return async () => {
    const state = await statePromise;
    const callback = new URL("/api/v1/azure-devops/oauth/callback", "http://localhost:38429");
    callback.searchParams.set("code", MOCK_CODE);
    callback.searchParams.set("state", state);
    return callback.toString();
  };
}

test.afterEach(async ({ apiClient, seedData }) => {
  await apiClient.deleteAzureDevOpsConfig(seedData.workspaceId);
  await apiClient.mockAzureDevOpsReset();
});

test("switches a PAT connection to a Microsoft account through the sign-in popup", async ({
  apiClient,
  seedData,
  testPage,
}) => {
  await apiClient.mockAzureDevOpsSeed(MOCK_STATE);
  await apiClient.setAzureDevOpsConfig(seedData.workspaceId, {
    organizationUrl: ORGANIZATION_URL,
    pat: "azure-test-pat",
  });
  await testPage.goto(settingsPath(seedData.workspaceId));
  await expect(testPage.getByTestId("azure-devops-pat")).toBeVisible();

  await selectMicrosoftAccount(testPage);
  const completed = waitForHttp(testPage, "POST", COMPLETE_PATH);
  await testPage.getByTestId("azure-devops-oauth-connect").click();
  expect((await completed).status()).toBe(200);

  await expect(testPage.getByTestId("azure-devops-oauth-account")).toContainText("Ada Reviewer");
  await expect(testPage.getByTestId("azure-devops-oauth-paste")).toHaveCount(0);
  const config = await apiClient.getAzureDevOpsConfig(seedData.workspaceId);
  expect(config?.authMethod).toBe("oauth");

  await testPage.reload();
  await expect(testPage.getByTestId("azure-devops-oauth-account")).toContainText("Ada Reviewer");
  await expect(testPage.getByTestId("azure-devops-save-button")).toBeEnabled();
});

test("a declined sign-in keeps the PAT connection", async ({ apiClient, seedData, testPage }) => {
  await apiClient.mockAzureDevOpsSeed({ ...MOCK_STATE, oauthError: "access_denied" });
  await apiClient.setAzureDevOpsConfig(seedData.workspaceId, {
    organizationUrl: ORGANIZATION_URL,
    pat: "azure-test-pat",
  });
  await testPage.goto(settingsPath(seedData.workspaceId));

  await selectMicrosoftAccount(testPage);
  await testPage.getByTestId("azure-devops-oauth-connect").click();

  await expect(testPage.getByText("Microsoft sign-in was cancelled.")).toBeVisible();
  await expect(testPage.getByTestId("azure-devops-oauth-account")).toHaveCount(0);
  const config = await apiClient.getAzureDevOpsConfig(seedData.workspaceId);
  expect(config?.authMethod).toBe("pat");
});

test("completes sign-in from a pasted redirect address", async ({
  apiClient,
  seedData,
  testPage,
}) => {
  await apiClient.mockAzureDevOpsSeed(MOCK_STATE);
  const callbackAddress = await holdAuthorizeRedirect(testPage.context());
  await testPage.goto(settingsPath(seedData.workspaceId));
  await testPage.getByTestId("azure-devops-organization").fill(ORGANIZATION_URL);

  await selectMicrosoftAccount(testPage);
  const popupPromise = testPage.waitForEvent("popup");
  await testPage.getByTestId("azure-devops-oauth-connect").click();
  await popupPromise;
  await expect(testPage.getByTestId("azure-devops-oauth-paste")).toBeVisible();

  await testPage.getByTestId("azure-devops-oauth-paste-url").fill(await callbackAddress());
  const completed = waitForHttp(testPage, "POST", COMPLETE_PATH);
  await testPage.getByTestId("azure-devops-oauth-paste-complete").click();
  expect((await completed).status()).toBe(200);

  await expect(testPage.getByTestId("azure-devops-oauth-account")).toContainText("Ada Reviewer");
  const config = await apiClient.getAzureDevOpsConfig(seedData.workspaceId);
  expect(config?.authMethod).toBe("oauth");
});

test("signs out of the Microsoft account and keeps the connection settings", async ({
  apiClient,
  seedData,
  testPage,
}) => {
  await apiClient.mockAzureDevOpsSeed(MOCK_STATE);
  await testPage.goto(settingsPath(seedData.workspaceId));
  await testPage.getByTestId("azure-devops-organization").fill(ORGANIZATION_URL);
  await selectMicrosoftAccount(testPage);
  const completed = waitForHttp(testPage, "POST", COMPLETE_PATH);
  await testPage.getByTestId("azure-devops-oauth-connect").click();
  expect((await completed).status()).toBe(200);
  await expect(testPage.getByTestId("azure-devops-oauth-account")).toContainText("Ada Reviewer");

  const signedOut = waitForHttp(testPage, "POST", /\/api\/v1\/azure-devops\/oauth\/sign-out$/);
  await testPage.getByTestId("azure-devops-oauth-sign-out").click();
  expect((await signedOut).status()).toBe(200);

  await expect(testPage.getByTestId("azure-devops-oauth-account")).toHaveCount(0);
  await expect(testPage.getByTestId("azure-devops-oauth-sign-out")).toHaveCount(0);
  await expect(testPage.getByTestId("azure-devops-oauth-connect")).toHaveText(
    /Sign in with Microsoft/,
  );
  const config = await apiClient.getAzureDevOpsConfig(seedData.workspaceId);
  expect(config).toMatchObject({
    authMethod: "oauth",
    hasSecret: false,
    organizationUrl: ORGANIZATION_URL,
  });
});

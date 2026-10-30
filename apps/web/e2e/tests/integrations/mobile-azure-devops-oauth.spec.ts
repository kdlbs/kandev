import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

// Covers AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.9: phone parity of the method
// choice, sign-in action, paste fallback, and signed-in identity.

const ORGANIZATION_URL = "https://dev.azure.com/acme";
const MOCK_STATE = {
  authenticated: true,
  user: { ok: true, id: "user-1", displayName: "Ada Reviewer" },
};

test.afterEach(async ({ apiClient, seedData }) => {
  await apiClient.deleteAzureDevOpsConfig(seedData.workspaceId);
  await apiClient.mockAzureDevOpsReset();
});

test("mobile Microsoft sign-in uses touch-sized actions and a paste fallback without overflow", async ({
  apiClient,
  seedData,
  testPage,
}) => {
  await apiClient.mockAzureDevOpsSeed(MOCK_STATE);
  let heldState = "";
  await testPage.context().route("**/api/v1/azure-devops/mock/oauth/authorize*", async (route) => {
    heldState = new URL(route.request().url()).searchParams.get("state") ?? "";
    await route.fulfill({ contentType: "text/html", body: "<!doctype html><title>hold</title>" });
  });
  await testPage.goto(
    `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/azure-devops`,
  );
  await testPage.getByTestId("azure-devops-organization").fill(ORGANIZATION_URL);
  await testPage.getByTestId("azure-devops-auth-method").click();
  await testPage.getByRole("option", { name: "Microsoft account", exact: true }).click();

  const connect = testPage.getByTestId("azure-devops-oauth-connect");
  await expect(connect).toBeEnabled();
  expect((await connect.boundingBox())!.height).toBeGreaterThanOrEqual(44);

  const authorize = testPage
    .context()
    .waitForEvent("request", (request) =>
      request.url().includes("/api/v1/azure-devops/mock/oauth/authorize"),
    );
  await connect.click();
  await authorize;
  await expect(testPage.getByTestId("azure-devops-oauth-paste")).toBeVisible();
  for (const testId of ["azure-devops-oauth-paste-complete", "azure-devops-oauth-paste-cancel"]) {
    expect((await testPage.getByTestId(testId).boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);

  const callback = new URL("http://localhost:38429/api/v1/azure-devops/oauth/callback");
  callback.searchParams.set("code", "mock-authorization-code");
  callback.searchParams.set("state", heldState);
  await testPage.getByTestId("azure-devops-oauth-paste-url").fill(callback.toString());
  const completed = waitForHttp(testPage, "POST", /\/api\/v1\/azure-devops\/oauth\/complete$/);
  await testPage.getByTestId("azure-devops-oauth-paste-complete").click();
  expect((await completed).status()).toBe(200);

  await expect(testPage.getByTestId("azure-devops-oauth-account")).toContainText("Ada Reviewer");
  const signOut = testPage.getByTestId("azure-devops-oauth-sign-out");
  await expect(signOut).toBeVisible();
  expect((await signOut.boundingBox())!.height).toBeGreaterThanOrEqual(44);
});

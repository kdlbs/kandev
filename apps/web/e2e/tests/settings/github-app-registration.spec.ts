import { test, expect } from "../../fixtures/test-base";

const SYSTEM_GITHUB_APP_PATH = "/settings/system/github-app";

test.describe("Deployment GitHub App onboarding", () => {
  test("validates setup, explains permissions, and hands the generated manifest to GitHub", async ({
    testPage,
  }, testInfo) => {
    await testPage.goto(SYSTEM_GITHUB_APP_PATH);

    const status = testPage.getByTestId("github-app-status");
    await expect(status).toHaveAttribute("data-source", "none");
    await expect(status).toHaveAttribute("data-state", "unconfigured");
    await expect(testPage.getByText("One App for this Kandev deployment")).toBeVisible();
    await expect(
      testPage.getByText(
        "An optional personal connection used for viewer-specific and human-attributed actions; agents never receive it.",
      ),
    ).toBeVisible();

    await testPage.getByTestId("github-app-create-button").click();
    await expect(testPage.getByText("Enter the GitHub organization login.")).toBeVisible();
    await expect(testPage.getByText("Enter a public HTTPS origin.")).toBeVisible();

    await testPage.getByLabel("Organization login").fill("acme");
    await testPage.getByLabel("Public Kandev URL").fill("http://localhost:8080/callback");
    await testPage.getByTestId("github-app-create-button").click();
    await expect(testPage.getByText("Enter a public HTTPS origin.")).toBeVisible();

    await testPage.getByTestId("github-app-permissions-button").click();
    const permissions = testPage.getByRole("dialog", { name: "GitHub App permissions" });
    await expect(permissions.getByText("Contents", { exact: true })).toBeVisible();
    await expect(permissions.getByText("Read and write repository content")).toBeVisible();
    await expect(permissions.getByText("Workflows", { exact: true })).toBeVisible();
    await expect(
      permissions.getByText(
        "Webhooks: installation, installation repositories, and GitHub App authorization.",
      ),
    ).toBeVisible();
    await permissions.getByRole("button", { name: "Close" }).click();

    let submittedManifest: Record<string, unknown> | undefined;
    await testPage.route(
      "https://github.com/organizations/acme/settings/apps/new",
      async (route) => {
        const fields = new URLSearchParams(route.request().postData() ?? "");
        const manifest = fields.get("manifest");
        expect(route.request().method()).toBe("POST");
        expect(manifest).not.toBeNull();
        submittedManifest = JSON.parse(manifest ?? "{}") as Record<string, unknown>;
        await route.fulfill({
          status: 200,
          contentType: "text/html",
          body: "<main><h1>Mock GitHub App confirmation</h1></main>",
        });
      },
    );

    await testPage.getByLabel("Public Kandev URL").fill("https://1.1.1.1");
    await testPage.getByTestId("github-app-create-button").click();
    const handoff = testPage.getByTestId("github-app-manifest-confirm");
    await expect(handoff).toBeVisible();
    await expect(handoff).toContainText("This setup request expires in one hour.");

    await testPage.screenshot({
      path: testInfo.outputPath("github-app-manifest-handoff-desktop.png"),
      fullPage: true,
    });

    await testPage.getByTestId("github-app-manifest-continue").click();
    await expect(
      testPage.getByRole("heading", { name: "Mock GitHub App confirmation" }),
    ).toBeVisible();
    expect(submittedManifest).toMatchObject({
      url: "https://1.1.1.1",
      hook_attributes: {
        url: "https://1.1.1.1/api/v1/github/app/webhook",
        active: true,
      },
      callback_urls: ["https://1.1.1.1/api/v1/github/personal-connection/callback"],
      setup_url: "https://1.1.1.1/api/v1/github/app/install/callback",
      default_permissions: {
        actions: "read",
        contents: "write",
        pull_requests: "write",
        workflows: "write",
      },
      default_events: ["installation", "installation_repositories", "github_app_authorization"],
    });
    expect(String(submittedManifest?.redirect_url)).toMatch(
      /^https:\/\/1\.1\.1\.1\/api\/v1\/github\/app\/registration\/callback\?state=/,
    );
  });

  test("shows callback failure without changing credentials and hot-enables a completed App", async ({
    testPage,
    apiClient,
  }) => {
    const replay = await apiClient.rawRequest(
      "GET",
      "/api/v1/github/app/registration/callback?state=replayed-state&code=replayed-code",
      undefined,
      { redirect: "manual" },
    );
    expect(replay.status).toBe(303);
    const replayLocation = replay.headers.get("location");
    expect(replayLocation).toContain("github_app_result=github_app_invalid_callback");

    await testPage.goto(replayLocation ?? SYSTEM_GITHUB_APP_PATH);
    await expect(testPage.getByTestId("github-app-callback-result")).toContainText(
      "GitHub App setup was not completed",
    );
    await expect(testPage.getByTestId("github-app-status")).toHaveAttribute(
      "data-state",
      "unconfigured",
    );

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
    const readyStatus = testPage.getByTestId("github-app-status");
    await expect(readyStatus).toHaveAttribute("data-state", "ready");
    await expect(readyStatus).toHaveAttribute("data-source", "managed");
    await expect(testPage.getByText("kandev-acme", { exact: true })).toBeVisible();
    await expect(testPage.getByTestId("github-app-webhook-status")).toContainText(
      "Waiting for webhook",
    );

    await testPage.getByTestId("github-app-workspace-handoff").click();
    await expect(testPage).toHaveURL(/\/settings\/integrations\/github$/);
    const automation = testPage.getByTestId("github-workspace-automation");
    await automation.getByRole("button", { name: "Connect GitHub" }).click();
    const connectionDialog = testPage.getByRole("dialog", { name: "Connect GitHub" });
    await connectionDialog.getByRole("combobox", { name: "Connection method" }).click();
    await testPage.getByRole("option", { name: "GitHub App", exact: true }).click();
    await expect(testPage.getByTestId("github-app-install-button")).toBeVisible();
  });

  test("keeps environment configuration read-only", async ({ testPage, apiClient }) => {
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

    const status = testPage.getByTestId("github-app-status");
    await expect(status).toHaveAttribute("data-source", "environment");
    await expect(testPage.getByText("Externally managed", { exact: true })).toBeVisible();
    await expect(testPage.getByTestId("github-app-environment-status")).toContainText(
      "Environment configuration has priority. This page cannot replace or remove it.",
    );
    await expect(testPage.getByTestId("github-app-remove-button")).toHaveCount(0);
    await expect(testPage.getByTestId("github-app-create-button")).toHaveCount(0);
  });

  test("blocks removal while a workspace is bound to the deployment App", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
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
    await testPage.goto(SYSTEM_GITHUB_APP_PATH);

    await testPage.getByTestId("github-app-remove-button").click();
    await testPage.getByTestId("github-app-remove-confirmation").fill("DELETE");
    await testPage.getByTestId("github-app-remove-confirm").click();

    await expect(testPage.getByText("deployment GitHub App is used by a workspace")).toBeVisible();
    await expect(testPage.getByText("kandev-acme", { exact: true })).toBeVisible();
    await expect(testPage.getByTestId("github-app-status")).toHaveAttribute("data-state", "ready");
  });
});

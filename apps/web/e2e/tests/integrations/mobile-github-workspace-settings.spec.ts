import { test, expect } from "../../fixtures/test-base";

test.describe("GitHub workspace settings on mobile", () => {
  test("explains and saves task Git credential inheritance", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);

    await testPage.getByRole("button", { name: "Explain task Git credentials" }).click();
    await expect(
      testPage.getByRole("dialog", { name: "How task Git credentials work" }),
    ).toContainText("GH_TOKEN or GITHUB_TOKEN");
    await prCapture.screenshot("mobile-task-git-credentials-help", {
      caption: "Mobile task Git credential explanation",
    });
    await testPage.keyboard.press("Escape");

    await testPage.getByRole("radio", { name: "Inherit executor Git credentials" }).click();
    await testPage.getByTestId("settings-floating-save").getByRole("button").click();
    await expect(testPage.getByText("Task Git credential settings saved")).toBeVisible({
      timeout: 10_000,
    });

    const response = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
    );
    expect(await response.json()).toMatchObject({ task_git_credentials_mode: "executor" });
  });

  test("explains repository scope below issue watches without requiring hover", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/integrations/github`);

    const issueWatchesHeading = testPage.getByRole("heading", { name: "Issue Watches" });
    const repositoryScopeHeading = testPage.getByRole("heading", {
      name: "Repository Scope",
      exact: true,
    });
    const scopeDescription = testPage.getByText(
      "Limits GitHub pull requests and issues shown or imported in this workspace.",
      { exact: true },
    );

    await expect(scopeDescription).toBeVisible();
    const scopeHelpButton = testPage.getByRole("button", { name: "Explain repository scope" });
    await expect(scopeHelpButton).toBeVisible();

    const [issueWatchesBox, repositoryScopeBox] = await Promise.all([
      issueWatchesHeading.boundingBox(),
      repositoryScopeHeading.boundingBox(),
    ]);
    expect(issueWatchesBox).not.toBeNull();
    expect(repositoryScopeBox).not.toBeNull();
    expect(repositoryScopeBox!.y).toBeGreaterThan(issueWatchesBox!.y);

    await scopeHelpButton.click();
    await expect(testPage.getByRole("dialog", { name: "Repository Scope" })).toContainText(
      "including My GitHub results and review and issue watches",
    );
  });
});

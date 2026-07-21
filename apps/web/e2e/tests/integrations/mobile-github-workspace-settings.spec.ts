import { test, expect } from "../../fixtures/test-base";

test.describe("GitHub workspace settings on mobile", () => {
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
    await expect(testPage.getByRole("button", { name: "Explain repository scope" })).toBeVisible();

    const [issueWatchesBox, repositoryScopeBox] = await Promise.all([
      issueWatchesHeading.boundingBox(),
      repositoryScopeHeading.boundingBox(),
    ]);
    expect(issueWatchesBox).not.toBeNull();
    expect(repositoryScopeBox).not.toBeNull();
    expect(repositoryScopeBox!.y).toBeGreaterThan(issueWatchesBox!.y);
  });
});

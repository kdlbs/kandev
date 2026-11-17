import { expect, test } from "../fixtures/test-base";
import { waitForHttp } from "../helpers/causal-waits";

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.3 .12
test("edits and reloads mixed GitHub repository scope on mobile", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("mobile-scope-user");
  await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
    source: "legacy_shared",
    status: "active",
  });
  await apiClient.mockGitHubAddOrgs([{ login: "acme" }]);
  await apiClient.mockGitHubAddRepos("outside", [
    { full_name: "outside/tool", owner: "outside", name: "tool" },
  ]);
  const initial = await apiClient.rawRequest("PUT", "/api/v1/github/workspace-settings", {
    workspace_id: seedData.workspaceId,
    repo_scope_mode: "repos",
    repo_scope_repos: [{ owner: "outside", name: "tool" }],
  });
  expect(initial.status).toBe(200);

  await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations/github`);
  const scope = testPage.getByTestId("github-repo-scope-section");
  await expect(scope.getByTestId("github-repository-chip")).toHaveText(["outside/tool"]);
  const [toggleBox, organizationAddBox, repositoryAddBox] = await Promise.all([
    scope.locator('label[for="github-scope-all-repositories"]').boundingBox(),
    scope.getByTestId("github-organization-add").boundingBox(),
    scope.getByTestId("github-repository-add").boundingBox(),
  ]);
  expect(toggleBox?.height).toBeGreaterThanOrEqual(44);
  expect(organizationAddBox?.height).toBeGreaterThanOrEqual(44);
  expect(repositoryAddBox?.height).toBeGreaterThanOrEqual(44);

  await scope.getByTestId("github-organization-add").tap();
  const input = scope.getByTestId("github-organization-input");
  await input.fill("acme");
  await input.press("Enter");
  await expect(scope.getByTestId("github-organization-chip")).toHaveText(["acme"]);

  const save = testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" });
  expect((await save.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  const settingsSaved = waitForHttp(testPage, "PUT", /^\/api\/v1\/github\/workspace-settings$/);
  await save.tap();
  expect((await settingsSaved).status()).toBe(200);
  await expect(testPage.getByText("GitHub workspace settings saved").last()).toBeVisible();
  await testPage.reload();
  await expect(scope.getByTestId("github-organization-chip")).toHaveText(["acme"]);
  await expect(scope.getByTestId("github-repository-chip")).toHaveText(["outside/tool"]);

  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
    ),
  ).toBe(false);
});

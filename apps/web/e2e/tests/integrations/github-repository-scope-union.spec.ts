import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import type { Page } from "@playwright/test";

// @covers AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.1 .2 .3 .9
test("saves owner and repository scope together", async ({ testPage, apiClient, seedData }) => {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("scope-user");
  await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
    source: "legacy_shared",
    status: "active",
  });
  await apiClient.mockGitHubAddOrgs([{ login: "acme" }]);
  await apiClient.mockGitHubAddRepos("outside", [
    { full_name: "outside/tool", owner: "outside", name: "tool" },
  ]);

  const legacyWrite = await apiClient.rawRequest("PUT", "/api/v1/github/workspace-settings", {
    workspace_id: seedData.workspaceId,
    repo_scope_mode: "repos",
    repo_scope_repos: [{ owner: "outside", name: "tool" }],
  });
  expect(legacyWrite.status).toBe(200);
  expect(await legacyWrite.json()).toMatchObject({
    repo_scope_mode: "selected",
    repo_scope_orgs: [],
    repo_scope_repos: [{ owner: "outside", name: "tool" }],
  });

  await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations/github`);
  const scope = testPage.getByTestId("github-repo-scope-section");
  await expect(scope.getByTestId("github-repository-chip")).toHaveText(["outside/tool"]);
  await addScopeOrganization(testPage, "acme");
  const selectedSettingsSaved = waitForHttp(
    testPage,
    "PUT",
    /^\/api\/v1\/github\/workspace-settings$/,
  );
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .click();
  expect((await selectedSettingsSaved).status()).toBe(200);
  await expect(testPage.getByText("GitHub workspace settings saved").last()).toBeVisible();

  const saved = await apiClient.rawRequest(
    "GET",
    `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
  );
  expect(await saved.json()).toMatchObject({
    repo_scope_mode: "selected",
    repo_scope_orgs: ["acme"],
    repo_scope_repos: [{ owner: "outside", name: "tool" }],
  });

  await testPage.reload();
  await expect(scope.getByTestId("github-organization-chip")).toHaveText(["acme"]);
  await expect(scope.getByTestId("github-repository-chip")).toHaveText(["outside/tool"]);

  const allRepositories = scope.getByRole("switch", { name: "All repositories" });
  await allRepositories.click();
  const allSettingsSaved = waitForHttp(testPage, "PUT", /^\/api\/v1\/github\/workspace-settings$/);
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .click();
  expect((await allSettingsSaved).status()).toBe(200);
  await expect(testPage.getByText("GitHub workspace settings saved").last()).toBeVisible();
  const allSettings = await apiClient.rawRequest(
    "GET",
    `/api/v1/github/workspace-settings?workspace_id=${seedData.workspaceId}`,
  );
  expect(await allSettings.json()).toMatchObject({
    repo_scope_mode: "all",
    repo_scope_orgs: [],
    repo_scope_repos: [],
  });
});

async function addScopeOrganization(page: Page, login: string): Promise<void> {
  const field = page.getByTestId("github-scope-orgs");
  await field.getByRole("button", { name: "Add organization" }).click();
  const input = field.getByTestId("github-organization-input");
  await input.fill(login);
  await input.press("Enter");
  await expect(field.getByTestId("github-organization-chip")).toHaveText([login]);
}

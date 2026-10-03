import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { ListRepositoriesResponse, Repository } from "../../../lib/types/http";

const REMOTE_OWNER = "mock-user";
const REMOTE_NAME = "settings-remote";
const REMOTE_FULL_NAME = `${REMOTE_OWNER}/${REMOTE_NAME}`;

/**
 * Registers a mock GitHub repository through Settings > Workspace >
 * Repositories > Add repository > Remote repository and proves it persisted
 * without creating a task. Covers
 * AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.1 through .5 and, with
 * `mobile`, .8.
 */
export async function addRemoteRepositoryFromSettings(options: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  mobile?: boolean;
}) {
  const { page, apiClient, seedData, mobile = false } = options;
  if (mobile) await page.setViewportSize({ width: 390, height: 844 });
  let savedRepository: Repository | undefined;

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser(REMOTE_OWNER);
  await apiClient.mockGitHubAddRepos(REMOTE_OWNER, [
    { full_name: REMOTE_FULL_NAME, owner: REMOTE_OWNER, name: REMOTE_NAME, private: false },
  ]);
  await apiClient.mockGitHubAddBranches(REMOTE_OWNER, REMOTE_NAME, [{ name: "main" }]);
  const tasksBefore = (await apiClient.listTasks(seedData.workspaceId)).tasks.length;

  try {
    await page.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    await page.getByRole("button", { name: "Add repository" }).click();
    // Opening the dialog mounts the picker, which lists the connected
    // provider's repositories; the option can only render after that answer.
    const reposListed = page.waitForResponse(
      (response) =>
        response.url().includes("/api/v1/github/repos") &&
        response.request().method() === "GET" &&
        response.ok(),
    );
    await page.getByRole("menuitem", { name: "Remote repository" }).click();

    const dialog = page.getByRole("dialog", { name: "Add Remote Repository" });
    await expect(dialog).toBeVisible();
    const confirm = dialog.getByRole("button", { name: "Add to workspace" });
    await expect(confirm).toBeDisabled();

    await reposListed;
    await dialog.getByTestId("remote-repo-chip-trigger").click();
    const option = page.getByTestId("remote-repo-option").filter({ hasText: REMOTE_FULL_NAME });
    await expect(option).toBeVisible();
    await option.first().click();
    await expect(confirm).toBeEnabled();

    const registerResponse = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/v1/workspaces/${seedData.workspaceId}/repositories/remote`) &&
        response.request().method() === "POST" &&
        response.ok(),
    );
    await confirm.click();
    const response = await registerResponse;
    expect(response.status()).toBe(201);
    savedRepository = (await response.json()) as Repository;
    await expect(dialog).toBeHidden();

    // The saved repository opens in its editor right away, so its name is an
    // input value here and plain card text only after the reload below. The
    // reload renders the list from the boot payload, so no request precedes
    // the card; the default assertion timeout covers hydration.
    await expect(page.getByPlaceholder("my-repo")).toHaveValue(REMOTE_FULL_NAME);
    await page.reload();
    await expect(page.locator('[data-slot="card"]', { hasText: REMOTE_FULL_NAME })).toBeVisible();

    const listResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/workspaces/${seedData.workspaceId}/repositories`,
    );
    expect(listResponse.ok).toBe(true);
    const listed = (await listResponse.json()) as ListRepositoriesResponse;
    expect(listed.repositories).toContainEqual(
      expect.objectContaining({
        id: savedRepository.id,
        provider: "github",
        provider_owner: REMOTE_OWNER,
        provider_name: REMOTE_NAME,
        default_branch: "main",
      }),
    );
    const tasksAfter = (await apiClient.listTasks(seedData.workspaceId)).tasks.length;
    expect(tasksAfter).toBe(tasksBefore);

    if (mobile) {
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
      ).toBe(false);
    }
  } finally {
    if (savedRepository) {
      await apiClient
        .rawRequest("DELETE", `/api/v1/repositories/${savedRepository.id}`)
        .catch(() => undefined);
    }
  }
}

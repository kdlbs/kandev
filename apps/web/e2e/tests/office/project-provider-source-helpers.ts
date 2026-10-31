import { expect, type Page } from "@playwright/test";
import { waitForHttp } from "../../helpers/causal-waits";

/**
 * Shared project-source scenarios for the provider-neutral Office suites.
 *
 * The project editor stores literal source strings, so these helpers drive the
 * real custom-entry picker rather than writing the URL through the API. That is
 * the only way the browser suite proves the stored project source survives a
 * reload and later reaches task creation unchanged.
 */

export const GITLAB_PROJECT_HOST = "https://gitlab.example.test:8443";
export const GITLAB_PROJECT_SOURCE = "https://gitlab.example.test:8443/group/subgroup/project";
export const GITLAB_PROJECT_ORIGIN = "https://gitlab.example.test:8443";

export const PLUGIN_PROJECT_SOURCE = "https://bitbucket.example.test/projects/TEAM/repos/fixture";
export const PLUGIN_PROJECT_CLONE_URL = "https://bitbucket.example.test/scm/TEAM/fixture.git";
export const PLUGIN_PROJECT_PROVIDER = "fixture-source-control";
export const PLUGIN_PROJECT_REPOSITORY_ID = "fixture-repository";

export function projectChipSelector(source: string): string {
  return `[data-testid="project-repo-chip"][data-repository-value="${source}"]`;
}

export async function openProject(page: Page, projectId: string): Promise<void> {
  await page.goto(`/office/projects/${projectId}`);
  await expect(page.getByTestId("project-repo-chips")).toBeVisible({ timeout: 15_000 });
}

/**
 * Adds a literal source through the project picker's custom entry and waits for
 * the persisted project update before returning.
 */
export async function addProjectSourceThroughPicker(
  page: Page,
  projectId: string,
  source: string,
): Promise<void> {
  await page.getByTestId("project-add-repository").click();
  const search = page.getByPlaceholder(/search or paste a url or path/i);
  await expect(search).toBeVisible({ timeout: 10_000 });
  await search.fill(source);
  const customEntry = page.getByTestId("project-add-custom");
  await expect(customEntry).toBeVisible({ timeout: 10_000 });
  const updated = waitForHttp(page, "PATCH", new RegExp(`/api/v1/office/projects/${projectId}$`));
  await customEntry.click();
  await updated;
  await expect(page.locator(projectChipSelector(source))).toBeVisible({ timeout: 10_000 });
}

export async function expectProjectSourceAfterReload(
  page: Page,
  projectId: string,
  source: string,
): Promise<void> {
  await openProject(page, projectId);
  await expect(page.locator(projectChipSelector(source))).toBeVisible({ timeout: 15_000 });
}

type RepositorySummary = {
  id: string;
  provider?: string;
  provider_host?: string;
  provider_repo_id?: string;
  provider_owner?: string;
  provider_name?: string;
  remote_url?: string;
  default_branch?: string;
};

export async function listWorkspaceRepositories(
  apiClient: { rawRequest: (method: string, path: string) => Promise<Response> },
  workspaceId: string,
): Promise<RepositorySummary[]> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/repositories`,
  );
  const body = await response.text();
  expect(response.ok, body).toBe(true);
  const parsed = JSON.parse(body) as { repositories?: RepositorySummary[] };
  return parsed.repositories ?? [];
}

export async function removeRepositoriesByProvider(
  apiClient: { rawRequest: (method: string, path: string) => Promise<Response> },
  workspaceId: string,
  matches: (repository: RepositorySummary) => boolean,
): Promise<void> {
  for (const repository of (await listWorkspaceRepositories(apiClient, workspaceId)).filter(
    matches,
  )) {
    const response = await apiClient.rawRequest("DELETE", `/api/v1/repositories/${repository.id}`);
    if (!response.ok && response.status !== 404) {
      throw new Error(`repository cleanup failed (${response.status}): ${await response.text()}`);
    }
  }
}

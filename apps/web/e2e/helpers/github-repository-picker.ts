import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";

type Seed = { workspaceId: string; workflowId: string; startStepId: string };

/** Repositories the mock GitHub client lists for its default `mock-user` login. */
export const ACCESSIBLE_REPOSITORIES = ["mock-user/alpha", "mock-user/beta", "mock-user/gamma"];

export async function seedAccessibleRepositories(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<void> {
  await apiClient.mockGitHubReset();
  // The picker lists repositories only for a workspace with an active GitHub connection.
  await apiClient.mockGitHubSetWorkspaceConnection(workspaceId, {
    source: "legacy_shared",
    status: "active",
  });
  await apiClient.mockGitHubAddRepos(
    "mock-user",
    ACCESSIBLE_REPOSITORIES.map((fullName) => {
      const [owner, name] = fullName.split("/");
      return { full_name: fullName, owner: owner!, name: name!, private: false };
    }),
  );
}

/** Seeds an automation with a repository-restricted "PR: opened" trigger and opens its editor. */
export async function openRestrictedPrAutomation(
  page: Page,
  apiClient: ApiClient,
  seed: Seed,
  name: string,
): Promise<Locator> {
  const automation = await apiClient.seedAutomation({
    workspaceId: seed.workspaceId,
    name,
    workflowId: seed.workflowId,
    workflowStepId: seed.startStepId,
  });
  await apiClient.seedTrigger({
    automationId: automation.id,
    type: "github_pr",
    config: { events: ["opened"], all_repos: false, repos: [] },
    enabled: true,
  });
  await page.goto(`/settings/workspace/${seed.workspaceId}/automations/${automation.id}`);
  await page.getByTestId("automation-editor").waitFor({ state: "visible" });
  return expandPrTriggerCard(page);
}

/** Seeded trigger cards render collapsed; expanding mounts the repository filter. */
export async function expandPrTriggerCard(page: Page): Promise<Locator> {
  const card = page.getByTestId("trigger-card-github_pr");
  await card
    .getByRole("button", { name: /PR: opened/i })
    .first()
    .click();
  await expect(card.getByTestId("github-repository-add")).toBeVisible();
  return card;
}

export function repositoryOptions(scope: Locator): Locator {
  return scope.getByTestId("remote-repo-option");
}

export function organizationOptions(scope: Locator): Locator {
  return scope.getByTestId("github-organization-option");
}

import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient, MockPR, QueueSessionIdentityInput } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { CreateTaskResponse } from "../../../lib/types/http";
import type {
  EntityReference,
  EntityReferenceSearchResponse,
} from "../../../lib/types/entity-reference";
import { entityReferenceMarkdown } from "../../../lib/entity-references/message-references";

export const PR_OWNER = "testorg";
export const PR_REPOSITORY = "e2e-pr-registration";
export const PR_NUMBER = 407;
export const PR_REPOSITORY_ID = "100407";
export const PR_TITLE = "E2E selected pull request";
export const PR_URL = `https://github.com/${PR_OWNER}/${PR_REPOSITORY}/pull/${PR_NUMBER}`;

export function pullRequestReference(): EntityReference {
  return {
    version: 1,
    ref: `mention:v1:github:pull_request:github.com%2F${PR_OWNER}%2F${PR_REPOSITORY}:PR_kwE2E407`,
    provider: "github",
    kind: "pull_request",
    id: "PR_kwE2E407",
    key: `${PR_OWNER}/${PR_REPOSITORY}#${PR_NUMBER}`,
    title: PR_TITLE,
    url: PR_URL,
    scope: `github.com/${PR_OWNER}/${PR_REPOSITORY}`,
  };
}

export function queuedContextReference(): EntityReference {
  return {
    ...pullRequestReference(),
    selection_id: "selection-queued-context",
    association_mode: "context_only",
  };
}

export async function prepareQueuedPullRequestContext(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<{ identity: QueueSessionIdentityInput; content: string }> {
  const identity = await apiClient.getQueueSessionIdentity(taskId, sessionId);
  await apiClient.setQueueAutoRun(identity, false);
  const reference = queuedContextReference();
  const content = entityReferenceMarkdown(reference);
  await apiClient.queueMessage(identity, content, undefined, [reference]);
  return { identity, content };
}

export async function reselectQueuedPullRequestForSave(page: Page, row: Locator): Promise<string> {
  await row.getByTitle("Edit queued message").click();
  const textarea = page.getByTestId("queue-edit-textarea");
  await expect(textarea).toBeVisible();
  const unchangedContent = await textarea.inputValue();
  await page.getByTestId("queue-reference-add").click();
  const search = page.getByTestId("queue-reference-search");
  await search.fill(PR_TITLE);
  const option = page.getByRole("option", {
    name: new RegExp(`#${PR_OWNER}/${PR_REPOSITORY}#${PR_NUMBER}.*${PR_TITLE}`),
  });
  await expect(option).toBeVisible({ timeout: 10_000 });
  await option.click();
  await expect(textarea).toHaveValue(unchangedContent);
  await expect(page.getByTestId("reference-registration-trigger")).toContainText("Link on save");
  return unchangedContent;
}

export async function preparePullRequestMentionTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<CreateTaskResponse> {
  await preparePullRequestMentionProvider(apiClient, seedData);

  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

export async function preparePullRequestMentionProvider(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<void> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("registration-e2e");
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "github",
    provider_repo_id: PR_REPOSITORY_ID,
    provider_host: "https://github.com",
    provider_owner: PR_OWNER,
    provider_name: PR_REPOSITORY,
  });

  const pr: MockPR = {
    number: PR_NUMBER,
    title: PR_TITLE,
    state: "open",
    head_branch: "feat/e2e-registration",
    base_branch: "main",
    author_login: "registration-e2e",
    repo_owner: PR_OWNER,
    repo_name: PR_REPOSITORY,
    base_repo_id: Number(PR_REPOSITORY_ID),
    html_url: PR_URL,
  };
  await apiClient.mockGitHubAddPRs([pr]);
}

export async function routePullRequestSearch(page: Page): Promise<void> {
  const reference = pullRequestReference();
  await page.route("**/api/v1/workspaces/*/mentions/search?*", async (route) => {
    const query = new URL(route.request().url()).searchParams.get("q") ?? "";
    const response: EntityReferenceSearchResponse = {
      query,
      groups: [
        {
          source: "github_pull_requests",
          provider: "github",
          kind: "pull_request",
          display_name: "GitHub pull requests",
          kind_label: "Pull request",
          status: "ok",
          results: [reference],
        },
      ],
    };
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(response),
    });
  });
}

export async function waitForFiniteAnimations(page: Page): Promise<void> {
  await page.evaluate(async () => {
    const animations = document.getAnimations().filter((animation) => {
      const iterations = animation.effect?.getComputedTiming().iterations ?? 1;
      return animation.playState === "running" && Number.isFinite(iterations);
    });
    await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
  });
}

export function entityReferencesFromMessage(message: {
  metadata?: Record<string, unknown>;
}): EntityReference[] {
  const references = message.metadata?.entity_references;
  if (!Array.isArray(references)) return [];
  return references.filter(
    (reference): reference is EntityReference =>
      typeof reference === "object" &&
      reference !== null &&
      "provider" in reference &&
      "kind" in reference &&
      "url" in reference,
  );
}

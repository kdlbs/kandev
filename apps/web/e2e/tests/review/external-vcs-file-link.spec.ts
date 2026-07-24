import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const GITHUB_OWNER = "testorg";
const GITHUB_REPOSITORY = "external-file-links";
const BASE_GITHUB_REPOSITORY = "external-file-links-base";
const PUBLISHED_FILE = "published-link.ts";
const EXISTING_FILE = "base-link.ts";
const UNTRACKED_FILE = "local-only-link.ts";

function initializeRepository(backend: BackendContext, slug: string): string {
  const repositoryPath = path.join(backend.tmpDir, "repos", slug);
  fs.mkdirSync(repositoryPath, { recursive: true });
  const gitEnvironment = makeGitEnv(backend.tmpDir);
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnvironment });
  fs.writeFileSync(path.join(repositoryPath, EXISTING_FILE), "export const base = true;\n");
  execFileSync("git", ["add", "-A"], { cwd: repositoryPath, env: gitEnvironment });
  execFileSync("git", ["commit", "-m", "seed provider repository"], {
    cwd: repositoryPath,
    env: gitEnvironment,
  });
  return repositoryPath;
}

async function createGitHubRepository(
  apiClient: ApiClient,
  workspaceId: string,
  repositoryPath: string,
  repositoryName = GITHUB_REPOSITORY,
) {
  const options = {
    name: repositoryName,
    provider: "github",
    provider_host: "https://github.com",
    provider_owner: GITHUB_OWNER,
    provider_name: repositoryName,
  };
  return apiClient.createRepository(workspaceId, repositoryPath, "main", options);
}

function trustedGitHubTaskRepository(repositoryName = GITHUB_REPOSITORY) {
  return {
    remote_url: `https://github.com/${GITHUB_OWNER}/${repositoryName}.git`,
    provider: "github",
    provider_owner: GITHUB_OWNER,
    provider_name: repositoryName,
    base_branch: "main",
  };
}

async function openExternalLink(page: Page, link: Locator, expectedURL: string): Promise<void> {
  await page.context().route("https://github.com/**", async (route) => {
    await route.fulfill({ status: 200, contentType: "text/html", body: "external file" });
  });
  const popupPromise = page.waitForEvent("popup");
  await link.click();
  const popup = await popupPromise;
  await popup.waitForLoadState("domcontentloaded");
  expect(popup.url()).toBe(expectedURL);
  await popup.close();
}

test.describe("External VCS file links", () => {
  test.describe.configure({ timeout: 120_000 });

  test("opens a linked pull request file on the published head branch", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repositoryPath = initializeRepository(backend, `external-link-pr-${Date.now()}`);
    await createGitHubRepository(apiClient, seedData.workspaceId, repositoryPath);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("reviewer");
    await apiClient.mockGitHubAddPRs([
      {
        number: 81,
        title: "External file link",
        state: "open",
        head_branch: "feature/external-file-link",
        base_branch: "main",
        author_login: "reviewer",
        repo_owner: GITHUB_OWNER,
        repo_name: GITHUB_REPOSITORY,
      },
    ]);
    await apiClient.mockGitHubAddPRFiles(GITHUB_OWNER, GITHUB_REPOSITORY, 81, [
      {
        filename: PUBLISHED_FILE,
        status: "modified",
        additions: 1,
        deletions: 1,
        patch: "@@ -1 +1 @@\n-export const published = false;\n+export const published = true;",
      },
    ]);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Published external file link",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [trustedGitHubTaskRepository()],
      },
    );
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: GITHUB_OWNER,
      repo: GITHUB_REPOSITORY,
      pr_number: 81,
      pr_url: `https://github.com/${GITHUB_OWNER}/${GITHUB_REPOSITORY}/pull/81`,
      pr_title: "External file link",
      head_branch: "feature/external-file-link",
      base_branch: "main",
      author_login: "reviewer",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await session.clickTab("Changes");
    await expect(
      session.changes.getByRole("button", { name: new RegExp(PUBLISHED_FILE) }),
    ).toBeVisible({ timeout: 20_000 });
    await session.changes.getByRole("button", { name: "Review", exact: true }).click();

    const review = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(review).toBeVisible();
    const fileHeader = review.getByTestId("review-file-header").filter({ hasText: PUBLISHED_FILE });
    const link = fileHeader.getByRole("link", { name: "Open file in GitHub" });
    const expectedURL =
      "https://github.com/testorg/external-file-links/blob/feature%2Fexternal-file-link/published-link.ts";
    await expect(link).toHaveAttribute("href", expectedURL);
    await expect(link).toHaveAttribute("target", "_blank");
    await openExternalLink(testPage, link, expectedURL);
  });

  test("uses the base branch for an existing file and omits an unpublished untracked file", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repositoryPath = initializeRepository(backend, `external-link-base-${Date.now()}`);
    await createGitHubRepository(
      apiClient,
      seedData.workspaceId,
      repositoryPath,
      BASE_GITHUB_REPOSITORY,
    );
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Base external file link",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [trustedGitHubTaskRepository(BASE_GITHUB_REPOSITORY)],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    const sessions = await apiClient.listTaskSessions(task.id);
    const taskCheckout =
      sessions.sessions[0]?.worktrees?.[0]?.worktree_path ||
      sessions.sessions[0]?.worktree_path ||
      repositoryPath;
    fs.writeFileSync(path.join(taskCheckout, UNTRACKED_FILE), "export const local = true;\n");

    await session.clickTab("Changes");
    await expect(
      testPage.getByTestId(`file-row-${UNTRACKED_FILE.replaceAll("/", "-")}`),
    ).toBeVisible({
      timeout: 20_000,
    });
    await session.clickTab("Files");
    const existingNode = session.files.locator(
      `[data-testid="file-tree-node"][data-path="${EXISTING_FILE}"]`,
    );
    await expect(existingNode).toBeVisible({ timeout: 15_000 });
    await existingNode.click();

    const baseLink = testPage.getByRole("link", { name: "Open file in GitHub" });
    const expectedBaseURL =
      "https://github.com/testorg/external-file-links-base/blob/main/base-link.ts";
    await expect(baseLink).toHaveAttribute("href", expectedBaseURL);
    await openExternalLink(testPage, baseLink, expectedBaseURL);

    await session.clickTab("Files");
    const untrackedNode = session.files.locator(
      `[data-testid="file-tree-node"][data-path="${UNTRACKED_FILE}"]`,
    );
    await expect(untrackedNode).toBeVisible({ timeout: 15_000 });
    await untrackedNode.click();
    await expect(testPage.getByRole("link", { name: "Open file in GitHub" })).toHaveCount(0);
  });
});

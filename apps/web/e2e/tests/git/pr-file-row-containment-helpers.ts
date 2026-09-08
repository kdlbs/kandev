import { expect, type Locator, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const LONG_PR_PATH =
  "src/review/PlaytreeCacheHandoverSeedRestorationCoordinatorWithAnExcessivelyLongName.kt";
export const LONG_PR_BASENAME = LONG_PR_PATH.slice(LONG_PR_PATH.lastIndexOf("/") + 1);
export const PR_DIFF_MARKER = "PR_FILE_ROW_CONTAINMENT_MARKER";

const PR_OWNER = "testorg";
const PR_REPO = "testrepo";
const PR_NUMBER = 4242;

export async function seedLongPRFileTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  const checkoutBranch = "main";

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  await apiClient.mockGitHubAddPRs([
    {
      number: PR_NUMBER,
      title: "Long PR file row containment",
      state: "open",
      head_branch: checkoutBranch,
      base_branch: "main",
      author_login: "test-user",
      repo_owner: PR_OWNER,
      repo_name: PR_REPO,
      additions: 123,
      deletions: 45,
    },
  ]);
  await apiClient.mockGitHubAddPRFiles(PR_OWNER, PR_REPO, PR_NUMBER, [
    {
      filename: LONG_PR_PATH,
      status: "modified",
      additions: 123,
      deletions: 45,
      patch: `@@ -1 +1 @@\n-old\n+${PR_DIFF_MARKER}`,
    },
  ]);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repositories: [{ repository_id: seedData.repositoryId, checkout_branch: checkoutBranch }],
    },
  );

  await apiClient.mockGitHubAssociateTaskPR({
    task_id: task.id,
    workspace_id: seedData.workspaceId,
    repository_id: seedData.repositoryId,
    owner: PR_OWNER,
    repo: PR_REPO,
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/${PR_OWNER}/${PR_REPO}/pull/${PR_NUMBER}`,
    pr_title: "Long PR file row containment",
    head_branch: checkoutBranch,
    base_branch: "main",
    author_login: "test-user",
    additions: 123,
    deletions: 45,
  });

  return task;
}

export async function expandPRChanges(scope: Locator, input: "click" | "tap") {
  const toggle = scope.getByTestId("pr-changes-section-collapse-toggle");
  await expect(toggle).toBeVisible();
  await expect
    .poll(async () => {
      if ((await toggle.getAttribute("aria-expanded")) === "true") return true;
      if (input === "tap") await toggle.tap();
      else await toggle.click();
      return (await toggle.getAttribute("aria-expanded")) === "true";
    })
    .toBe(true);
}

export function longPRFileRow(scope: Locator) {
  return scope.locator(`[data-changes-file="${LONG_PR_PATH}"]`);
}

export async function expectLongPRRowContained(row: Locator) {
  const filename = row.getByText(LONG_PR_BASENAME, { exact: true });
  const additions = row.getByText("+123", { exact: true });
  const deletions = row.getByText("-45", { exact: true });
  const status = row.locator('[data-file-status="modified"]');

  await expect(row).toBeVisible();
  await expect(filename).toBeVisible();
  await expect(additions).toBeVisible();
  await expect(deletions).toBeVisible();
  await expect(status).toBeVisible();

  await expect
    .poll(async () => {
      const [rowBox, filenameBox, additionsBox, deletionsBox, statusBox] = await Promise.all([
        row.boundingBox(),
        filename.boundingBox(),
        additions.boundingBox(),
        deletions.boundingBox(),
        status.boundingBox(),
      ]);
      if (!rowBox || !filenameBox || !additionsBox || !deletionsBox || !statusBox) return false;
      return (
        filenameBox.x + filenameBox.width <= additionsBox.x &&
        additionsBox.x >= rowBox.x &&
        deletionsBox.x + deletionsBox.width <= statusBox.x &&
        statusBox.x + statusBox.width <= rowBox.x + rowBox.width
      );
    })
    .toBe(true);

  const statusCenterHitsStatus = await status.evaluate((element) => {
    const box = element.getBoundingClientRect();
    const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    return hit === element || element.contains(hit);
  });
  expect(statusCenterHitsStatus).toBe(true);

  await expect(row.locator(`button[title="${LONG_PR_PATH}"]`)).toHaveAttribute(
    "title",
    LONG_PR_PATH,
  );
}

export async function expectNoHorizontalOverflow(element: Locator) {
  await expect
    .poll(() => element.evaluate((node) => node.scrollWidth <= node.clientWidth))
    .toBe(true);
}

export async function waitForDiffMarker(page: Page) {
  await page.waitForFunction((marker) => {
    for (const container of document.querySelectorAll("diffs-container")) {
      if (container.shadowRoot?.textContent?.includes(marker)) return true;
    }
    return false;
  }, PR_DIFF_MARKER);
}

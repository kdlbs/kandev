import { expect, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const PR_NUMBER = 701;

type ReviewLayout = {
  canonicalGroupId: string | null;
  canonicalPRKey: string | null;
  keyedPanelIds: string[];
  rightTopOrder: string[];
};

async function createTaskWithSession(apiClient: ApiClient, seedData: SeedData, title: string) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error(`${title} did not create a session`);
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 45_000, message: `Waiting for ${title} session to settle` },
    )
    .toBe(true);
  return task;
}

async function openTask(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForDockviewReady();
  return session;
}

async function readReviewLayout(page: Page): Promise<ReviewLayout | null> {
  return page.evaluate(() => {
    type Panel = {
      id: string;
      params?: { prKey?: string };
      group?: { id?: string; panels?: Panel[] };
    };
    type Api = { getPanel: (id: string) => Panel | undefined; panels: Panel[] };
    const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    if (!api) return null;
    const canonical = api.getPanel("pr-detail");
    const files = api.getPanel("files");
    return {
      canonicalGroupId: canonical?.group?.id ?? null,
      canonicalPRKey: canonical?.params?.prKey ?? null,
      keyedPanelIds: api.panels
        .filter((panel) => panel.id.startsWith("pr-detail|"))
        .map((panel) => panel.id),
      rightTopOrder: files?.group?.panels?.map((panel) => panel.id) ?? [],
    };
  });
}

async function seedMockPR(apiClient: ApiClient): Promise<void> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  await apiClient.mockGitHubAddPRs([
    {
      number: PR_NUMBER,
      title: "Layout-owned review",
      state: "open",
      head_branch: "feat/pr-details-layout",
      base_branch: "main",
      author_login: "test-user",
      repo_owner: "testorg",
      repo_name: "testrepo",
      additions: 10,
      deletions: 2,
    },
  ]);
}

async function linkPR(apiClient: ApiClient, taskId: string): Promise<void> {
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: "testorg",
    repo: "testrepo",
    pr_number: PR_NUMBER,
    pr_url: `https://github.com/testorg/testrepo/pull/${PR_NUMBER}`,
    pr_title: "Layout-owned review",
    head_branch: "feat/pr-details-layout",
    base_branch: "main",
    author_login: "test-user",
    additions: 10,
    deletions: 2,
  });
}

test.describe("PR Details layout panel", () => {
  test("keeps the Default panel in Files/Changes and syncs linked review content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedMockPR(apiClient);
    const task = await createTaskWithSession(apiClient, seedData, "PR Details default layout");
    const session = await openTask(testPage, task.id);

    await expect
      .poll(() => readReviewLayout(testPage), { timeout: 15_000 })
      .toMatchObject({
        canonicalGroupId: "group-right-top",
        rightTopOrder: ["files", "changes", "pr-detail"],
      });

    await expect(session.prDetailTab()).toBeVisible();
    await session.prDetailTab().click();
    await expect(testPage.getByText("No pull request linked to this session.")).toBeVisible();

    await linkPR(apiClient, task.id);
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(() => readReviewLayout(testPage), { timeout: 15_000 })
      .toMatchObject({ canonicalPRKey: `testorg/testrepo/${PR_NUMBER}`, keyedPanelIds: [] });

    await session.prTopbarButton().click();
    await expect(session.prDetailPanel()).toBeVisible();
    await expect.poll(() => readReviewLayout(testPage)).toMatchObject({ keyedPanelIds: [] });
  });

  test("does not recreate PR Details after the user removes it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedMockPR(apiClient);
    const task = await createTaskWithSession(apiClient, seedData, "Removed PR Details layout");
    const session = await openTask(testPage, task.id);

    const panelTab = session.prDetailTab();
    await expect(panelTab).toBeVisible();
    await panelTab.hover();
    await panelTab.locator(".dv-default-tab-action").click();
    await expect(panelTab).not.toBeVisible();
    await expect
      .poll(() => readReviewLayout(testPage))
      .toMatchObject({
        canonicalGroupId: null,
        keyedPanelIds: [],
      });

    await linkPR(apiClient, task.id);
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await testPage.waitForTimeout(500);
    await expect
      .poll(() => readReviewLayout(testPage))
      .toMatchObject({
        canonicalGroupId: null,
        keyedPanelIds: [],
      });
  });
});

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

function sessionTabWrapper(page: Page, sessionId: string) {
  return page.locator(".dv-tab", {
    has: page.getByTestId(`session-tab-${sessionId}`),
  });
}

test.describe("PR Details layout panel", () => {
  test("adds linked review content beside Agent without changing focus", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedMockPR(apiClient);
    const task = await createTaskWithSession(apiClient, seedData, "PR Details default layout");
    const session = await openTask(testPage, task.id);

    await expect(session.prDetailTab()).toHaveCount(0);
    await expect
      .poll(() => readReviewLayout(testPage), { timeout: 15_000 })
      .toMatchObject({ canonicalGroupId: null, rightTopOrder: ["files", "changes"] });

    await linkPR(apiClient, task.id);
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });
    await expect(sessionTabWrapper(testPage, task.session_id!)).toHaveClass(/dv-active-tab/);
    await expect
      .poll(() => readReviewLayout(testPage), { timeout: 15_000 })
      .toMatchObject({
        canonicalGroupId: "group-center",
        canonicalPRKey: `testorg/testrepo/${PR_NUMBER}`,
        keyedPanelIds: [],
      });

    await session.prDetailTab().click();
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

    await expect(session.prDetailTab()).toHaveCount(0);

    await linkPR(apiClient, task.id);
    await expect(session.prTopbarButton()).toBeVisible({ timeout: 15_000 });
    const panelTab = session.prDetailTab();
    await expect(panelTab).toBeVisible({ timeout: 15_000 });
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
    await expect
      .poll(() => readReviewLayout(testPage))
      .toMatchObject({
        canonicalGroupId: null,
        keyedPanelIds: [],
      });
  });

  test("restores each task's selected center tab after a PR Details round trip", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedMockPR(apiClient);
    const taskA = await createTaskWithSession(apiClient, seedData, "Active Agent PR round trip A");
    const taskB = await createTaskWithSession(apiClient, seedData, "Active Agent PR round trip B");
    await linkPR(apiClient, taskA.id);

    const session = await openTask(testPage, taskA.id);
    const agentTab = sessionTabWrapper(testPage, taskA.session_id!);
    await expect(agentTab).toHaveClass(/dv-active-tab/);

    // Make Agent the last selected center tab after explicitly visiting PR Details.
    await session.prDetailTab().click();
    await expect(session.prDetailPanel()).toBeVisible();
    await session.clickSessionChatTab();
    await expect(agentTab).toHaveClass(/dv-active-tab/);

    await session.clickTaskInSidebar("Active Agent PR round trip B");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskB.id), {
      timeout: 15_000,
    });
    await session.waitForDockviewReady();

    await session.clickTaskInSidebar("Active Agent PR round trip A");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskA.id), {
      timeout: 15_000,
    });
    await session.waitForDockviewReady();
    await expect(agentTab).toHaveClass(/dv-active-tab/, { timeout: 15_000 });
    await expect(testPage.getByTestId(`session-tab-${taskA.session_id}`)).toHaveCount(1);

    // A deliberate PR Details selection remains deliberate across the same round trip.
    await session.prDetailTab().click();
    const reviewTab = testPage.locator(".dv-tab", { has: session.prDetailTab() });
    await expect(reviewTab).toHaveClass(/dv-active-tab/);
    await session.clickTaskInSidebar("Active Agent PR round trip B");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskB.id), {
      timeout: 15_000,
    });
    await session.waitForDockviewReady();
    await session.clickTaskInSidebar("Active Agent PR round trip A");
    await expect(testPage).toHaveURL((url) => url.pathname.includes(taskA.id), {
      timeout: 15_000,
    });
    await session.waitForDockviewReady();
    await expect(testPage.locator(".dv-tab", { has: session.prDetailTab() })).toHaveClass(
      /dv-active-tab/,
      {
        timeout: 15_000,
      },
    );
    await expect(agentTab).not.toHaveClass(/dv-active-tab/);
  });
});

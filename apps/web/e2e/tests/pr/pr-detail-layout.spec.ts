import { expect, type Locator, type Page } from "@playwright/test";
import { test, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { getDockviewGroupWidth, resizeColumnViaSplitview } from "../../helpers/dockview-resize";
import {
  assertTextWrapsNaturallyWithoutHorizontalOverflow,
  requireBox,
  type ElementBox,
} from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const PR_NUMBER = 701;
const RESPONSIVE_PR_NUMBER = 702;
const RESPONSIVE_PR_TITLE =
  "Keep this long pull request title visible while review and merge controls wrap below it at narrow widths";
const RESPONSIVE_PR_URL = `https://github.com/testorg/testrepo/pull/${RESPONSIVE_PR_NUMBER}`;

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

function verticalOverlap(first: ElementBox, second: ElementBox): number {
  return Math.min(first.y + first.height, second.y + second.height) - Math.max(first.y, second.y);
}

async function textLineCount(locator: Locator): Promise<number> {
  return locator.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    return Array.from(range.getClientRects()).filter((rect) => rect.width > 0).length;
  });
}

async function singleLineTextWidth(locator: Locator, label: string): Promise<number> {
  const widths = await locator.evaluate((element) => {
    const range = document.createRange();
    range.selectNodeContents(element);
    return Array.from(range.getClientRects(), (rect) => rect.width).filter((width) => width > 0);
  });
  expect(widths, `${label} should render on exactly one line`).toHaveLength(1);
  const [width] = widths;
  if (width === undefined) throw new Error(`${label}: text has no rendered line rectangles`);
  return width;
}

async function resizeReviewDetailTo(page: Page, targetWidth: number): Promise<void> {
  const detail = page.getByTestId("change-request-detail");
  const currentDetailWidth = await detail.evaluate((element) =>
    Math.round(element.getBoundingClientRect().width),
  );
  const currentRightWidth = await getDockviewGroupWidth(page, "files");
  await resizeColumnViaSplitview(
    page,
    "right",
    Math.round(currentRightWidth + currentDetailWidth - targetWidth),
  );
  await expect
    .poll(() => detail.evaluate((element) => Math.round(element.getBoundingClientRect().width)), {
      message: `change request detail did not settle at ${targetWidth}px`,
    })
    .toBe(targetWidth);
}

async function expectCenterHit(locator: Locator, label: string): Promise<void> {
  await expect(
    locator.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const hit = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2);
      return hit === element || (hit !== null && element.contains(hit));
    }),
    `${label} center is not a usable hit target`,
  ).resolves.toBe(true);
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

  test("moves PR actions below before they force the title to wrap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 1600, height: 900 });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const task = await createTaskWithSession(apiClient, seedData, "Responsive PR Details header");
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: "testorg",
      repo: "testrepo",
      pr_number: RESPONSIVE_PR_NUMBER,
      pr_url: RESPONSIVE_PR_URL,
      pr_title: RESPONSIVE_PR_TITLE,
      head_branch: "feat/responsive-pr-header",
      base_branch: "main",
      author_login: "another-user",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
      review_count: 1,
      pending_review_count: 0,
      required_reviews: 1,
      checks_total: 1,
      checks_passing: 1,
    });
    await apiClient.mockGitHubSeedPRFeedback({
      owner: "testorg",
      repo: "testrepo",
      pr_number: RESPONSIVE_PR_NUMBER,
      checks: [
        {
          name: "Frontend tests",
          status: "completed",
          conclusion: "success",
        },
      ],
      reviews: [
        {
          id: 1,
          author: "code-owner",
          state: "APPROVED",
          created_at: "2026-08-14T10:00:00Z",
        },
      ],
    });
    await expect
      .poll(() => apiClient.getTaskPR(task.id), {
        message: "responsive PR fixture did not reach merge-ready state",
      })
      .toMatchObject({
        state: "open",
        review_state: "approved",
        checks_state: "success",
        mergeable_state: "clean",
        review_count: 1,
        required_reviews: 1,
      });

    const session = await openTask(testPage, task.id);
    await expect(session.prDetailTab()).toBeVisible({ timeout: 15_000 });
    await session.prDetailTab().click();
    const detail = testPage.getByTestId("change-request-detail");
    const title = detail.getByTestId("change-request-detail-title");
    const actions = detail.getByTestId("change-request-detail-actions");
    const approve = detail.getByTestId("pr-approve-button");
    const merge = detail.getByTestId("pr-merge-button");
    const refresh = detail.getByRole("button", { name: "Refresh" });
    await expect(title).toBeVisible();
    await expect(approve).toBeVisible();
    await expect(merge).toBeVisible();
    await expect(refresh).toBeVisible();

    await resizeReviewDetailTo(testPage, 1200);
    const [wideDetailBox, wideTitleBox, wideActionsBox, wideTitleTextWidth] = await Promise.all([
      requireBox(detail, "wide detail"),
      requireBox(title, "wide title"),
      requireBox(actions, "wide actions"),
      singleLineTextWidth(title, "wide title"),
    ]);
    expect(await textLineCount(title), "wide title should fit on one line").toBe(1);
    expect(
      verticalOverlap(wideTitleBox, wideActionsBox),
      "actions may stay inline when the title remains one line",
    ).toBeGreaterThan(0);

    const inlineOverhead = wideDetailBox.width - wideTitleBox.width - wideActionsBox.width;
    const squeezedWidth = Math.ceil(wideTitleTextWidth + inlineOverhead + wideActionsBox.width / 2);
    await resizeReviewDetailTo(testPage, squeezedWidth);
    const [squeezedDetailBox, squeezedTitleBox, squeezedActionsBox] = await Promise.all([
      requireBox(detail, "squeezed detail"),
      requireBox(title, "squeezed title"),
      requireBox(actions, "squeezed actions"),
    ]);
    expect(
      await textLineCount(title),
      "actions should move below before they force the title onto another line",
    ).toBe(1);
    expect(
      squeezedActionsBox.y,
      "squeezed actions should sit below the title",
    ).toBeGreaterThanOrEqual(squeezedTitleBox.y + squeezedTitleBox.height);
    expect(
      squeezedTitleBox.width,
      "squeezed title should recover the full row",
    ).toBeGreaterThanOrEqual(squeezedDetailBox.width - 25);

    await resizeReviewDetailTo(testPage, 600);
    const [detailBox, titleBox, approveBox, mergeBox, refreshBox] = await Promise.all([
      requireBox(detail, "narrow detail"),
      requireBox(title, "narrow title"),
      requireBox(approve, "narrow approve action"),
      requireBox(merge, "narrow merge action"),
      requireBox(refresh, "narrow refresh action"),
    ]);
    expect(approveBox.y, "approval should start below the title").toBeGreaterThanOrEqual(
      titleBox.y + titleBox.height,
    );
    expect(mergeBox.y, "merge should share the action row").toBeCloseTo(approveBox.y, 0);
    expect(refreshBox.y, "refresh should share the action row").toBeCloseTo(approveBox.y, 0);
    expect(titleBox.width, "title should own the full padded row").toBeGreaterThanOrEqual(
      detailBox.width - 25,
    );
    await assertTextWrapsNaturallyWithoutHorizontalOverflow(title, "narrow PR title");
    expect(approveBox.x, "narrow actions should share the title's leading edge").toBeCloseTo(
      titleBox.x,
      0,
    );
    expect(approveBox.x).toBeLessThan(mergeBox.x);
    expect(mergeBox.x).toBeLessThan(refreshBox.x);
    expect(refreshBox.x + refreshBox.width).toBeLessThanOrEqual(detailBox.x + detailBox.width);
    await expectCenterHit(approve, "approve action");
    await expectCenterHit(merge, "merge action");
  });
});

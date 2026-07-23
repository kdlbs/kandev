import { test, expect } from "../../fixtures/test-base";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { GITLAB_PROJECT, seedGitLabReview } from "../../helpers/gitlab";
import { GitLabPage } from "../../pages/gitlab-page";
import { GitLabSettingsPage } from "../../pages/gitlab-settings-page";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

async function expectTouchTarget(locator: ReturnType<GitLabPage["mrRow"]>, label: string) {
  const box = await locator.boundingBox();
  expect(box, `${label} has no bounding box`).not.toBeNull();
  if (!box) return;
  expect(box.width, `${label} width`).toBeGreaterThanOrEqual(44);
  expect(box.height, `${label} height`).toBeGreaterThanOrEqual(44);
}

test.describe("Mobile GitLab parity", () => {
  test("browses, quick launches, reviews, subscribes, and unlinks without overflow", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    await seedGitLabReview(apiClient, seedData.workspaceId, 111, "Mobile GitLab review");
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: "https://gitlab.example.test",
      provider_owner: "platform",
      provider_name: "kandev",
    });

    const gitlab = new GitLabPage(testPage);
    await gitlab.goto();
    await expect(gitlab.mobileFiltersButton).toBeVisible();
    await expect(gitlab.mobileSidebar).toBeHidden();
    await expectTouchTarget(gitlab.mobileFiltersButton, "GitLab filters button");
    await gitlab.mobileFiltersButton.tap();
    await expect(gitlab.mobileSidebar).toBeVisible();
    await gitlab.mobileSidebar.evaluate(async (element) => {
      await Promise.allSettled(element.getAnimations().map((animation) => animation.finished));
    });
    await assertLocatorWithinViewportX(gitlab.mobileSidebar, "GitLab mobile filters");
    await gitlab.mobileSidebar.getByRole("button", { name: "Review requested" }).tap();
    await expect(gitlab.mobileSidebar).toBeHidden();
    await expect(gitlab.mrRow(111)).toContainText("Mobile GitLab review");
    await assertNoDocumentHorizontalOverflow(testPage, "GitLab mobile browse");

    await gitlab.startMRTask(111);
    const mrButton = testPage.getByTestId("mr-topbar-button");
    await expect(mrButton).toHaveAttribute("data-mr-iid", "111");
    await expectTouchTarget(mrButton, "linked MR button");
    await gitlab.openLinkedMR(111);

    const panel = testPage.getByTestId("mr-detail-panel").last();
    await expect(panel.getByText("Mobile GitLab review", { exact: true })).toBeVisible();
    await assertLocatorWithinViewportX(panel, "mobile MR review panel");
    await prCapture.screenshot("mobile-merge-request-review", {
      caption: "Mobile GitLab merge request review with touch-sized task controls",
    });
    await panel.getByRole("button", { name: "Approve", exact: true }).tap();
    await expect(testPage.getByText("Merge request approved", { exact: true })).toBeVisible();
    await panel.getByRole("button", { name: "Subscribe to GitLab notifications" }).tap();
    await expect(
      testPage.getByText("Subscribed to GitLab notifications", { exact: true }),
    ).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "GitLab mobile review");

    await panel.getByRole("button", { name: "Unlink merge request" }).tap();
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-mr-review-panel")).toHaveCount(0);
    await expect(testPage.getByTestId("session-chat")).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Chat", exact: true })).toHaveClass(
      /text-primary/,
    );
    await expect
      .poll(() =>
        testPage.evaluate(
          "window.__KANDEV_E2E_STORE__?.getState().mobileSession.activePanelBySessionId[window.__KANDEV_E2E_STORE__?.getState().tasks.activeSessionId ?? '']",
        ),
      )
      .toBe("chat");
  });

  test("watch controls remain touch sized and persist a pause", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.configureGitLab(seedData.workspaceId);
    const watch = await apiClient.createGitLabReviewWatch({
      workspace_id: seedData.workspaceId,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      projects: [{ path: GITLAB_PROJECT }],
    });
    await expect
      .poll(async () => {
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/gitlab/watches/review?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
        );
        const body = (await response.json()) as {
          watches: Array<{ id: string; last_polled_at?: string }>;
        };
        return body.watches.find((item) => item.id === watch.id)?.last_polled_at ?? null;
      })
      .not.toBeNull();
    await seedGitLabReview(apiClient, seedData.workspaceId, 112, "Mobile watch dispatch MR");

    const settings = new GitLabSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const mobileList = settings.reviewWatches.getByTestId("gitlab-watch-mobile-list");
    await expect(mobileList).toBeVisible();
    await expect(mobileList.getByText(GITLAB_PROJECT, { exact: true })).toBeVisible();
    const pause = mobileList.getByRole("button", { name: "Pause watch" });
    const check = mobileList.getByRole("button", { name: "Check now" });
    await expectTouchTarget(pause, "pause watch button");
    await expectTouchTarget(check, "check watch button");
    await check.tap();
    await expect(testPage.getByText(/Found 1 matching merge request/)).toBeVisible();
    const taskTitle = `[${GITLAB_PROJECT}!112] Mobile watch dispatch MR`;
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(taskTitle)).toHaveCount(1, {
      timeout: 20_000,
    });
    await settings.goto(seedData.workspaceId);
    await check.tap();
    await expect(
      testPage.getByText("No new merge requests matched", { exact: true }),
    ).toBeVisible();
    await kanban.goto();
    await expect(kanban.taskCardByTitle(taskTitle)).toHaveCount(1);

    await settings.goto(seedData.workspaceId);
    await pause.tap();
    const save = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: /save changes/i });
    await expect(save).toBeEnabled();
    await save.tap();
    await expect(mobileList.getByText("Paused", { exact: true })).toBeVisible();
    await expect(check).toBeDisabled();
    await assertLocatorWithinViewportX(mobileList, "mobile watch list");
    await assertNoDocumentHorizontalOverflow(testPage, "GitLab mobile watch settings");
  });

  test("creates and auto-links an MR with GitLab terminology", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const remoteURL = `${backend.baseUrl}/${GITLAB_PROJECT}.git`;
    await apiClient.configureGitLab(seedData.workspaceId, backend.baseUrl);
    await apiClient.configureGitLabRepositoryRemote(seedData.repositoryId, remoteURL);
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: backend.baseUrl,
      provider_owner: "platform",
      provider_name: "kandev",
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Create mobile GitLab MR",
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("Mobile GitLab creation task did not return a session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({
      timeout: 45_000,
    });
    const actions = testPage.getByTestId("mobile-git-actions");
    await expectTouchTarget(actions, "mobile Git actions");
    await actions.tap();
    await testPage.getByRole("menuitem", { name: "Create MR", exact: true }).tap();
    const dialog = testPage.getByRole("dialog", { name: "Create merge request" });
    await expect(dialog).toBeVisible();
    await assertLocatorWithinViewportX(dialog, "mobile create MR dialog");
    await dialog
      .getByRole("textbox", { name: "Merge Request title", exact: true })
      .fill("Mobile runtime-created GitLab MR");
    await dialog
      .getByRole("textbox", { name: "Description", exact: true })
      .fill("Created from the mobile GitLab flow.");
    const draft = dialog.getByLabel("Create as draft");
    await expect(draft).toBeChecked();
    await dialog.getByRole("button", { name: "Create MR", exact: true }).tap();

    await expect
      .poll(async () => {
        try {
          return (await apiClient.getGitLabPushRecord(seedData.repositoryId)).args;
        } catch {
          return "";
        }
      })
      .toBe("push --set-upstream origin HEAD");
    const mrButton = testPage.getByTestId("mr-topbar-button");
    await expect(mrButton).toHaveAttribute("data-mr-iid", "100", { timeout: 120_000 });
    await expectTouchTarget(mrButton, "mobile auto-linked MR");
    await testPage.reload();
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute("data-mr-iid", "100", {
      timeout: 60_000,
    });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile created MR task");
  });
});

import { test, expect } from "../../fixtures/office-fixture";
import { selectAgentReplyTextWithTouch } from "../chat/agent-message-comments-helpers";
import {
  restoreSelectionCommentVisualViewport,
  setSelectionCommentVisualViewport,
} from "../../helpers/selection-comment-visual-viewport";

const FINAL_TRANSCRIPT_LINE = "Final transcript paragraph available above the composer.";
const COMMENT = [
  "The plan asks QA to review the release path.",
  ...Array.from({ length: 16 }, (_, index) => `Office transcript paragraph ${index + 1}.`),
  FINAL_TRANSCRIPT_LINE,
].join("\n\n");
const EXTRACT = "review the release path";

test.describe("Office thread extract comments on mobile", () => {
  test("the drawer shortcut adds a remark when Run is unavailable", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    test.setTimeout(90_000);
    const task = await apiClient.createTask(officeSeed.workspaceId, "Mobile thread extract", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.seedComment({
      taskId: task.id,
      authorType: "agent",
      authorId: officeSeed.agentId,
      body: COMMENT,
      source: "session",
    });

    await testPage.setViewportSize({ width: 600, height: 960 });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByTestId("office-task-title")).toHaveText("Mobile thread extract", {
      timeout: 20_000,
    });
    const body = testPage.locator("[data-agent-message-body]").filter({ hasText: COMMENT });
    await expect(body).toBeVisible({ timeout: 20_000 });
    await selectAgentReplyTextWithTouch(body, EXTRACT);
    const trigger = testPage.getByTestId("agent-message-comment-trigger");
    const dock = testPage.getByTestId("selection-comment-dock");
    const owner = body.locator("xpath=ancestor::div[contains(@class,'overflow-y-auto')][1]");
    const composer = body
      .locator("xpath=ancestor::*[@data-testid='task-chat-root'][1]")
      .locator("textarea");
    const [dockBox, ownerBox, composerBox] = await Promise.all([
      dock.boundingBox(),
      owner.boundingBox(),
      composer.boundingBox(),
    ]);
    expect(dockBox).not.toBeNull();
    expect(ownerBox).not.toBeNull();
    expect(composerBox).not.toBeNull();
    if (!dockBox || !ownerBox || !composerBox) throw new Error("Expected Office dock geometry");
    expect(dockBox.x).toBeGreaterThanOrEqual(ownerBox.x - 1);
    expect(dockBox.x + dockBox.width).toBeLessThanOrEqual(ownerBox.x + ownerBox.width + 1);
    expect(dockBox.y + dockBox.height).toBeLessThanOrEqual(composerBox.y + 1);

    const finalLine = body.getByText(FINAL_TRANSCRIPT_LINE, { exact: true });
    await owner.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await finalLine.scrollIntoViewIfNeeded();
    const [normalDockBox, normalFinalLineBox, visibleComposerBox] = await Promise.all([
      dock.boundingBox(),
      finalLine.boundingBox(),
      composer.boundingBox(),
    ]);
    expect(normalDockBox).not.toBeNull();
    expect(normalFinalLineBox).not.toBeNull();
    expect(visibleComposerBox).not.toBeNull();
    if (!normalDockBox || !normalFinalLineBox || !visibleComposerBox) {
      throw new Error("Expected Office transcript and composer geometry");
    }
    expect(visibleComposerBox.y).toBeLessThan(testPage.viewportSize()?.height ?? 0);
    expect(visibleComposerBox.y + visibleComposerBox.height).toBeLessThanOrEqual(
      testPage.viewportSize()?.height ?? 0,
    );
    expect(normalFinalLineBox.y + normalFinalLineBox.height).toBeLessThanOrEqual(
      normalDockBox.y + 1,
    );

    const beforeViewportChange = await owner.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return { innerHeight: window.innerHeight, top: rect.top, bottom: rect.bottom };
    });
    await setSelectionCommentVisualViewport(testPage, {
      height: Math.max(320, Math.floor(beforeViewportChange.innerHeight * 0.62)),
      offsetTop: 24,
    });
    const afterViewportChange = await owner.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return { innerHeight: window.innerHeight, top: rect.top, bottom: rect.bottom };
    });
    expect(afterViewportChange.innerHeight).toBe(beforeViewportChange.innerHeight);
    expect(afterViewportChange.top).toBeCloseTo(beforeViewportChange.top, 1);
    expect(afterViewportChange.bottom).toBeCloseTo(beforeViewportChange.bottom, 1);
    await owner.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await finalLine.scrollIntoViewIfNeeded();
    const [clippedDockBox, finalLineBox] = await Promise.all([
      dock.boundingBox(),
      finalLine.boundingBox(),
    ]);
    expect(clippedDockBox).not.toBeNull();
    expect(finalLineBox).not.toBeNull();
    if (!clippedDockBox || !finalLineBox) throw new Error("Expected clipped transcript geometry");
    expect(finalLineBox.y + finalLineBox.height).toBeLessThanOrEqual(clippedDockBox.y + 1);
    await trigger.tap();

    const drawer = testPage.getByTestId("agent-message-comment-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("agent-message-comment-run")).toHaveCount(0);
    const input = drawer.getByTestId("agent-message-comment-input");
    await input.fill("Please confirm the owner.");
    await input.press("Control+Shift+Enter");

    await expect(drawer).toBeHidden();
    await expect(testPage.getByTestId("composer-context-row")).toContainText(EXTRACT);
    await restoreSelectionCommentVisualViewport(testPage);
  });

  test("read-only tasks do not open a new extract composer", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Read-only thread extract", {
      workflow_id: officeSeed.workflowId,
    });
    await apiClient.updateTaskState(task.id, "CANCELLED");
    await apiClient.seedComment({
      taskId: task.id,
      authorType: "agent",
      authorId: officeSeed.agentId,
      body: COMMENT,
      source: "session",
    });

    await testPage.setViewportSize({ width: 600, height: 960 });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByTestId("office-task-title")).toHaveText("Read-only thread extract", {
      timeout: 20_000,
    });
    const body = testPage.locator("[data-agent-message-body]").filter({ hasText: COMMENT });
    await expect(body).toBeVisible();
    await selectAgentReplyTextWithTouch(body, EXTRACT);

    await expect(testPage.getByTestId("agent-message-comment-trigger")).toHaveCount(0);
    await expect(testPage.getByPlaceholder("Add a comment...")).toHaveCount(0);
    await restoreSelectionCommentVisualViewport(testPage);
  });
});

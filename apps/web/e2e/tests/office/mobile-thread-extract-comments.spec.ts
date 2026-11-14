import { test, expect } from "../../fixtures/office-fixture";
import { selectAgentReplyText } from "../chat/agent-message-comments-helpers";

const COMMENT = "The plan asks QA to review the release path.";
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
    await selectAgentReplyText(body, EXTRACT);
    await testPage.getByTestId("agent-message-comment-trigger").tap();

    const drawer = testPage.getByTestId("agent-message-comment-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.getByTestId("agent-message-comment-run")).toHaveCount(0);
    const input = drawer.getByTestId("agent-message-comment-input");
    await input.fill("Please confirm the owner.");
    await input.press("Control+Shift+Enter");

    await expect(drawer).toBeHidden();
    await expect(testPage.getByTestId("composer-context-row")).toContainText(EXTRACT);
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
    await selectAgentReplyText(body, EXTRACT);

    await expect(testPage.getByTestId("agent-message-comment-trigger")).toHaveCount(0);
    await expect(testPage.getByPlaceholder("Add a comment...")).toHaveCount(0);
  });
});

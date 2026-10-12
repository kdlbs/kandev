// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import {
  seedWorkflowResetFailure,
  WORKFLOW_RESET_ERROR_DETAILS,
  WORKFLOW_RESET_ERROR_MESSAGE,
} from "./workflow-reset-error-helpers";

test.describe("mobile: workflow reset failure recovery", () => {
  test("shows, dismisses, and remembers the reset failure notice", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { task, sessionId } = await seedWorkflowResetFailure(
      apiClient,
      seedData,
      `Mobile Workflow Reset Failure ${Date.now()}`,
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const notice = session.activeChat().getByTestId("last-agent-error-notice");
    await expect(notice).toBeVisible({ timeout: 15_000 });
    await expect(notice).toContainText(WORKFLOW_RESET_ERROR_MESSAGE);
    await notice.locator("summary").tap();
    await expect(notice).toContainText(WORKFLOW_RESET_ERROR_DETAILS);
    await assertNoDocumentHorizontalOverflow(
      testPage,
      "mobile expanded workflow reset failure notice",
    );

    const dismissButton = notice.getByRole("button", { name: "Hide previous agent error" });
    const buttonBox = await dismissButton.boundingBox();
    expect(buttonBox).not.toBeNull();
    expect(buttonBox!.height).toBeGreaterThanOrEqual(44);
    expect(buttonBox!.width).toBeGreaterThanOrEqual(44);

    await testPage.reload();
    await session.waitForLoad();
    const reloadedNotice = session.activeChat().getByTestId("last-agent-error-notice");
    await expect(reloadedNotice).toBeVisible({ timeout: 15_000 });
    await reloadedNotice.locator("summary").tap();
    await expect(reloadedNotice).toContainText(WORKFLOW_RESET_ERROR_DETAILS);
    await assertNoDocumentHorizontalOverflow(
      testPage,
      "mobile reloaded expanded workflow reset failure notice",
    );

    await reloadedNotice.getByRole("button", { name: "Hide previous agent error" }).tap();
    await expect(reloadedNotice).toBeHidden();
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const error = sessions.find((item) => item.id === sessionId)?.metadata?.last_agent_error as
          | { dismissed_at?: string }
          | undefined;
        return error?.dismissed_at ?? "";
      })
      .not.toBe("");

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.activeChat().getByTestId("last-agent-error-notice")).toBeHidden();
  });
});

import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  RECOVERY_CHECK,
  RECOVERY_CLOSE,
  RECOVERY_OPEN,
  RECOVERY_RETRY,
  RECOVERY_STATUS,
  expectCreateReady,
  expectOneRecoveryTask,
  expectOpeningPrompt,
  fillRecoveryDraft,
  installLookupMisses,
  installLostResponseRoute,
  openMobileCreateDialog,
  seedRecoveryWorkflow,
  selectRecoveryWorkflow,
  type CapturedCreate,
} from "./task-create-recovery-helpers";

useRegularMode();

test("phone keeps one native task through a lost response with reachable recovery actions", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  test.setTimeout(120_000);
  const title = `Recovery phone ${testInfo.workerIndex}-${testInfo.repeatEachIndex}-${Date.now()}`;
  const prompt = "Recover this phone draft without a second task.";
  const { workflowId } = await seedRecoveryWorkflow(apiClient, seedData, `Recovery phone ${title}`);
  const captured: CapturedCreate[] = [];
  await installLostResponseRoute(testPage, captured);
  await installLookupMisses(testPage);

  const dialog = await openMobileCreateDialog(testPage);
  await selectRecoveryWorkflow(dialog, workflowId);
  await fillRecoveryDraft(dialog, title, prompt);
  await expectCreateReady(dialog);
  await dialog.getByTestId("submit-start-agent").click();
  await expect(dialog.getByTestId(RECOVERY_STATUS)).toBeVisible();

  const check = dialog.getByTestId(RECOVERY_CHECK);
  const retry = dialog.getByTestId(RECOVERY_RETRY);
  const close = dialog.getByTestId(RECOVERY_CLOSE);
  await expect(check).toBeVisible();
  await expect(retry).toBeVisible();
  await expect(close).toBeVisible();

  for (const action of [check, retry, close]) {
    const box = await action.boundingBox();
    expect(box, "every recovery action needs a measured phone target").not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeLessThanOrEqual((await testPage.viewportSize())!.width);
  }

  const statusBox = await dialog.getByTestId(RECOVERY_STATUS).boundingBox();
  const checkBox = await check.boundingBox();
  const retryBox = await retry.boundingBox();
  const closeBox = await close.boundingBox();
  expect(statusBox!.y + statusBox!.height).toBeLessThanOrEqual(checkBox!.y);
  expect(checkBox!.y).toBeLessThan(retryBox!.y);
  expect(retryBox!.y).toBeLessThan(closeBox!.y);

  const overflow = await testPage.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(overflow).toBeLessThanOrEqual(1);

  await prCapture.screenshot("task-create-recovery-phone", {
    caption: "Phone recovery actions stay above the safe area with 44px targets.",
  });

  let postCount = 1;
  testPage.on("request", (request) => {
    if (request.method() === "POST" && new URL(request.url()).pathname === "/api/v1/tasks") {
      postCount += 1;
    }
  });
  await testPage.reload();
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileFab.tap();
  const reopened = testPage.getByTestId("create-task-dialog");
  await expect(reopened.getByTestId(RECOVERY_STATUS)).toBeVisible();
  expect(postCount, "phone restoration must not create another task").toBe(1);

  const retryResponse = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
  await reopened.getByTestId(RECOVERY_RETRY).click();
  await retryResponse;
  await expect(reopened.getByTestId(RECOVERY_STATUS)).toContainText("This task was created.");
  await expect(reopened.getByTestId(RECOVERY_OPEN)).toBeVisible();
  expect(postCount).toBe(2);

  await reopened.getByTestId(RECOVERY_OPEN).click();
  await expect(reopened).toBeHidden();
  await expectOneRecoveryTask(apiClient, seedData.workspaceId, title, captured[0]!.id);
  await expectOpeningPrompt(apiClient, captured[0]!.id, prompt);
});

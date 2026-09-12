import { test, expect } from "../../fixtures/test-base";
import { assertNoHorizontalOverflow } from "../../helpers/session-stream-overload";
import { seedSecondaryClarificationTask } from "../../helpers/clarification";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  captureThreadSettings,
  capturePresentation,
  seedThreadPresentation,
  startPresentationThread,
} from "./threads-presentation-helpers";

// @covers AC-UI-THREADS-DECK-004.6, AC-UI-THREADS-DECK-005.7, AC-UI-THREADS-DECK-005.8, AC-UI-THREADS-DECK-005.10
test("keeps the touch composer visible and restores drafts through single-chat navigation", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(180_000);
  const original = await captureThreadSettings(apiClient);
  try {
    const first = await startPresentationThread(testPage, apiClient, seedData, "A touch composer");
    const second = await startPresentationThread(testPage, apiClient, seedData, "B touch composer");
    await seedThreadPresentation(apiClient, { layout: "grid", autoHideComposer: true });
    await testPage.goto("/threads");
    const board = testPage.getByTestId("threads-board");
    const a = testPage.getByTestId(`thread-column-${first.id}`);
    const b = testPage.getByTestId(`thread-column-${second.id}`);
    const editor = a.getByTestId("chat-input-editor");
    await expect(editor).toBeVisible();
    await expect(board.getByTestId("session-chat")).toHaveCount(1);
    await expect(board.getByTestId("collapse-composer")).toHaveCount(0);
    await editor.tap();
    await editor.fill("My mobile draft");
    await a.getByTestId("thread-picker-trigger").tap();
    await testPage.getByTestId(`thread-picker-row-${second.id}`).tap();
    await expect(a.getByTestId("session-chat")).toHaveCount(0);
    await expect(b.getByTestId("chat-input-editor")).toBeVisible();
    await b.getByTestId("thread-picker-trigger").tap();
    await testPage.getByTestId(`thread-picker-row-${first.id}`).tap();
    await expect(testPage.getByRole("dialog", { name: "Choose thread", exact: true })).toBeHidden();
    await waitForFiniteAnimations(testPage.locator("body"));
    await expect(testPage.getByTestId("thread-swipe-cue")).toHaveText("1/2");
    await expect(b.getByTestId("session-chat")).toHaveCount(0);
    await expect(editor).toHaveText("My mobile draft");
    await testPage.setViewportSize({ width: 393, height: 500 });
    await expect(editor).toBeVisible();
    expect((await a.locator(".chat-message-list").boundingBox())!.height).toBeGreaterThanOrEqual(
      79,
    );
    const send = a.getByTestId("submit-message-button");
    expect((await send.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await send.tap();
    await expect(
      a.locator(".chat-message-list").getByText("My mobile draft", { exact: true }),
    ).toBeVisible();
    await expect(editor).toBeEmpty();
    await assertNoHorizontalOverflow(testPage);
    await capturePresentation(testPage, testInfo, "phone-visible-composer-short-height");
    await testPage.setViewportSize({ width: 900, height: 1100 });
    await expect(board).toHaveAttribute("data-layout", "grid");
    await expect(a.getByTestId("chat-input-editor")).toBeVisible();
    await expect(b.getByTestId("chat-input-editor")).toBeVisible();
    await expect(board.getByTestId("collapse-composer")).toHaveCount(0);
    expect((await captureThreadSettings(apiClient)).thread_views[0].auto_hide_composer).toBe(true);
    await capturePresentation(testPage, testInfo, "tablet-visible-composers");
    await assertNoHorizontalOverflow(testPage);
  } finally {
    await apiClient.saveUserSettings(original);
  }
});

// @covers AC-UI-THREADS-DECK-005.4, AC-UI-THREADS-DECK-005.8
test("keeps required answers reachable above a short mobile viewport", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const original = await captureThreadSettings(apiClient);
  try {
    const task = await seedSecondaryClarificationTask(apiClient, seedData, "Touch required answer");
    await seedThreadPresentation(apiClient, { layout: "grid", autoHideComposer: true });
    await testPage.setViewportSize({ width: 393, height: 500 });
    await testPage.goto(`/threads?taskId=${task.id}&sessionId=${task.clarificationSessionId}`);
    const tile = testPage.getByTestId(`thread-column-${task.id}`);
    const question = tile.getByTestId("clarification-overlay-container");
    await expect(question).toBeVisible();
    const answer = question.getByTestId("clarification-option").filter({ hasText: "PostgreSQL" });
    await expect(answer).toBeVisible();
    expect((await answer.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    expect((await tile.locator(".chat-message-list").boundingBox())!.height).toBeGreaterThanOrEqual(
      79,
    );
    await answer.tap();
    await expect(question).toHaveCount(0);
    await expect(tile.getByTestId("chat-input-editor")).toBeVisible();
    await expect(testPage.getByTestId("session-chat")).toHaveCount(1);
    await assertNoHorizontalOverflow(testPage);
  } finally {
    await apiClient.saveUserSettings(original);
  }
});

// @covers AC-UI-THREADS-DECK-005.4, AC-UI-THREADS-DECK-005.5, AC-UI-THREADS-DECK-005.7
test("keeps native model, cancellation and attachment controls usable by touch", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const original = await captureThreadSettings(apiClient);
  try {
    const task = await startPresentationThread(
      testPage,
      apiClient,
      seedData,
      "Touch agent controls",
    );
    await seedThreadPresentation(apiClient, { layout: "grid", autoHideComposer: true });
    await testPage.goto("/threads");
    const tile = testPage.getByTestId(`thread-column-${task.id}`);
    const model = tile.getByRole("button", { name: "Session model settings" });
    await model.tap();
    await testPage.getByRole("option", { name: /Mock Smart/ }).tap();
    await expect(model).toContainText("Mock Smart");
    const editor = tile.getByTestId("chat-input-editor");
    await editor.fill("/slow 8s");
    await tile.getByTestId("submit-message-button").tap();
    const cancel = tile.getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible();
    expect((await cancel.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await cancel.tap();
    await expect(cancel).toBeDisabled();
    await expect(editor).toBeVisible();
    await expect(cancel).toBeHidden({ timeout: 15_000 });
    await tile.locator('input[type="file"]').setInputFiles({
      name: "touch-notes.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("Touch attachment draft"),
    });
    await expect(tile.getByText("touch-notes.txt", { exact: false })).toBeVisible();
    await expect(tile.getByTestId("submit-message-button")).toBeEnabled();
    await expect(tile.getByTestId("collapse-composer")).toHaveCount(0);
    await assertNoHorizontalOverflow(testPage);
  } finally {
    await apiClient.saveUserSettings(original);
  }
});

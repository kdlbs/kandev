import { test, expect, type Page } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  createPromptErrorFixture,
  expectPromptErrorRow,
  GENERIC_PROMPT_ERROR,
  promptEventCount,
  waitForPromptError,
} from "../../helpers/acp-prompt-error-continuity";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(300_000);
test.use({ trace: "retain-on-failure" });

async function openTaskSwitcher(page: Page) {
  await page.getByRole("button", { name: /^Open task switcher:/ }).tap();
}

function visibleTaskRow(page: Page, title: string) {
  return page
    .locator('[data-testid="sidebar-task-item"]:visible')
    .filter({ hasText: title })
    .first();
}

async function expectMobileBadge(page: Page, title: string, visible: boolean) {
  await openTaskSwitcher(page);
  const row = visibleTaskRow(page, title);
  await expect(row).toBeVisible();
  const icon = row.getByTestId("task-agent-error-icon");
  if (visible) {
    await expect(icon).toBeVisible({ timeout: 15_000 });
  } else {
    await expect(icon).toHaveCount(0);
  }
  await page.keyboard.press("Escape");
  await expect(row).toBeHidden({ timeout: 10_000 });
}

test("phone: generic prompt error shows one inline error with a usable composer", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createPromptErrorFixture(backend, apiClient, seedData);
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await waitForPromptError(apiClient, fixture.sessionId);

    const row = await expectPromptErrorRow(session);
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    await expect(session.recoveryResumeButton()).toHaveCount(0);
    await expect(session.recoveryFreshButton()).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage);

    await expectMobileBadge(testPage, fixture.taskTitle, true);

    const details = row.locator("details");
    await expect(details).toHaveCount(1);
    await details.locator("summary").tap();
    await expect(details.locator("pre")).toContainText("acp_prompt");
    await assertNoDocumentHorizontalOverflow(testPage);

    const dismiss = row.getByTestId("turn-error-dismiss-button");
    await expect(dismiss).toBeVisible();
    const box = await dismiss.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(43.5);
    await dismiss.tap();
    await expect(dismiss).toHaveCount(0);
    await expect(row).toContainText(GENERIC_PROMPT_ERROR);
    await expectMobileBadge(testPage, fixture.taskTitle, false);
  } finally {
    await fixture.dispose();
  }
});

test("phone: visible per-row task actions expose Dismiss turn error", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createPromptErrorFixture(backend, apiClient, seedData);
  try {
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await waitForPromptError(apiClient, fixture.sessionId);

    await openTaskSwitcher(testPage);
    const row = visibleTaskRow(testPage, fixture.taskTitle);
    await expect(row).toBeVisible();
    const trigger = row.locator("button.mobile-task-actions-button");
    await expect(trigger).toBeVisible();
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox?.width ?? 0).toBeGreaterThanOrEqual(43.5);
    await trigger.tap();
    const dismiss = testPage.getByTestId("task-context-dismiss-turn-error");
    await expect(dismiss).toBeVisible();
    const box = await dismiss.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(43.5);
    await dismiss.tap();

    await expect(visibleTaskRow(testPage, fixture.taskTitle)).toBeVisible();
    await expect(
      visibleTaskRow(testPage, fixture.taskTitle).getByTestId("task-agent-error-icon"),
    ).toHaveCount(0);
    await testPage.keyboard.press("Escape");
    const errorRow = session.activeChat().getByTestId("session-recovery-action-message");
    await expect(errorRow).toBeVisible();
    await expect(errorRow.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
    expect(promptEventCount(fixture.tracePath)).toBe(1);
  } finally {
    await fixture.dispose();
  }
});

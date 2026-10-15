import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  createRestartScenario,
  captureRestartBaseline,
  assertRestartRetained,
} from "./session-tab-restart-helpers";

// @covers AC-TASKS-SESSION-TAB-RESTART-001.1, AC-TASKS-SESSION-TAB-RESTART-001.2, AC-TASKS-SESSION-TAB-RESTART-001.5
test("restarts a running agent from its tab without losing history", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const fixture = await createRestartScenario(testPage, apiClient, seedData);
  await fixture.session.sendMessage('e2e:message("Restart running witness")\ne2e:delay(60000)');
  await expect(
    fixture.session.chat.getByText("Restart running witness", { exact: true }),
  ).toBeVisible();
  const before = await captureRestartBaseline(apiClient, fixture.task.id, fixture.sessionId);
  expect(before.target.state).toBe("RUNNING");
  const stop = fixture.wire.waitForResponse("session.stop");
  const launch = fixture.wire.waitForResponse("session.launch", { timeoutAfter: stop });
  const wireStart = fixture.traffic.frames.length;
  await fixture.session.sessionTabBySessionId(fixture.sessionId).click({ button: "right" });
  await testPage.getByRole("menuitem", { name: "Restart session", exact: true }).click();
  await Promise.all([stop, launch]);
  await assertRestartRetained(apiClient, fixture.task.id, fixture.sessionId, before);
  await expect(
    fixture.session.chat.getByText("simple mock response", { exact: false }),
  ).toBeVisible();
  const requests = fixture.traffic.frames
    .slice(wireStart)
    .filter(
      (f) => f.direction === "sent" && ["session.stop", "session.launch"].includes(f.action ?? ""),
    );
  expect(requests.map((f) => [f.action, f.sessionId])).toEqual([
    ["session.stop", fixture.sessionId],
    ["session.launch", fixture.sessionId],
  ]);
});

test("restarts a non-active tab while preserving the selected session and draft", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const fixture = await createRestartScenario(testPage, apiClient, seedData);
  await fixture.session.openNewSessionDialog();
  await fixture.session.newSessionPromptInput().fill("/e2e:simple-message");
  await fixture.session.newSessionStartButton().click();
  await expect(fixture.session.newSessionDialog()).not.toBeVisible();
  await expect
    .poll(
      async () =>
        (await apiClient.listTaskSessions(fixture.task.id)).sessions.filter(
          (s) => s.state === "WAITING_FOR_INPUT",
        ).length,
      { message: "Waiting for both sessions to become idle", timeout: 30_000 },
    )
    .toBe(2);
  const sessions = (await apiClient.listTaskSessions(fixture.task.id)).sessions;
  const sibling = sessions.find((s) => s.id !== fixture.sessionId)!;
  await fixture.session.sessionTabBySessionId(sibling.id).click();
  const editor = await fixture.session.fillMessage("Keep this unsent draft");
  const before = await captureRestartBaseline(apiClient, fixture.task.id, fixture.sessionId);
  const stop = fixture.wire.waitForResponse("session.stop");
  const launch = fixture.wire.waitForResponse("session.launch", { timeoutAfter: stop });
  await fixture.session.sessionTabBySessionId(fixture.sessionId).click({ button: "right" });
  await testPage.getByRole("menuitem", { name: "Restart session", exact: true }).click();
  await Promise.all([stop, launch]);
  await assertRestartRetained(apiClient, fixture.task.id, fixture.sessionId, before);
  await expect(editor).toContainText("Keep this unsent draft");
  await expect
    .poll(() => testPage.evaluate("window.__KANDEV_E2E_STORE__.getState().tasks.activeSessionId"))
    .toBe(sibling.id);
});

test("offers keyboard restart from the preview tab and preserves phone boundary sizing", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const fixture = await createRestartScenario(testPage, apiClient, seedData);
  const baseline = (await apiClient.getUserSettings()).settings.enable_preview_on_click;
  try {
    await apiClient.saveUserSettings({ enable_preview_on_click: true });
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workflowId);
    await kanban.taskCard(fixture.task.id).click();
    const tab = testPage.getByTestId(`preview-session-tab-${fixture.sessionId}`);
    await expect(tab).toBeVisible();
    const before = await captureRestartBaseline(apiClient, fixture.task.id, fixture.sessionId);
    const stop = fixture.wire.waitForResponse("session.stop");
    const launch = fixture.wire.waitForResponse("session.launch", { timeoutAfter: stop });
    await tab.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Restart session", exact: true }).focus();
    await testPage.keyboard.press("Enter");
    await Promise.all([stop, launch]);
    await assertRestartRetained(apiClient, fixture.task.id, fixture.sessionId, before);
    await testPage.goto(`/t/${fixture.task.id}`);
    for (const width of [700, 767, 768]) {
      await testPage.setViewportSize({ width, height: 900 });
      if (width < 768) {
        await testPage.getByTestId("mobile-sessions-pill").click();
        const row = testPage.getByTestId(`mobile-session-row-${fixture.sessionId}`);
        await row.getByRole("button", { name: "Session actions" }).click();
        const action = testPage.getByRole("menuitem", { name: "Restart session", exact: true });
        await waitForFiniteAnimations(testPage.getByRole("menu"));
        expect((await action.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        await testPage.keyboard.press("Escape");
        await expect(testPage.getByRole("menu")).not.toBeVisible();
        await row.click();
        await expect(
          testPage.getByRole("dialog", { name: "Sessions", exact: true }),
        ).not.toBeVisible();
      } else {
        await expect(fixture.session.sessionTabBySessionId(fixture.sessionId)).toBeVisible();
      }
      await assertNoDocumentHorizontalOverflow(testPage, `restart at ${width}px`);
    }
  } finally {
    await apiClient.saveUserSettings({ enable_preview_on_click: baseline });
  }
});

import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { routeNavigationResponses } from "../../helpers/navigation-response-hold";
import {
  createRestartScenario,
  captureRestartBaseline,
  assertRestartRetained,
  assertRestartDataRetained,
  assertRestartSubmissionBlocked,
} from "./session-tab-restart-helpers";

test.use({ trace: "retain-on-failure" });

// @covers AC-TASKS-SESSION-TAB-RESTART-001.6
test("blocks sending throughout restart while keeping the draft editable", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 1600, height: 1000 });
  const gate = await routeNavigationResponses(testPage);
  const fixture = await createRestartScenario(testPage, apiClient, seedData);
  await apiClient.launchSession({
    task_id: fixture.task.id,
    agent_profile_id: seedData.agentProfileId,
    prompt: "/e2e:simple-message",
  });
  await expect
    .poll(
      async () =>
        (await apiClient.listTaskSessions(fixture.task.id)).sessions.filter(
          (session) => session.state === "WAITING_FOR_INPUT",
        ).length,
      { message: "Waiting for the sibling session to become ready", timeout: 30_000 },
    )
    .toBe(2);
  await fixture.session.sessionTabBySessionId(fixture.sessionId).click();
  const before = await captureRestartBaseline(apiClient, fixture.task.id, fixture.sessionId);
  const draftText = "Keep this draft while the session restarts";
  await fixture.session.fillMessage(draftText);
  gate.hold((request) => request.action === "session.stop");
  try {
    await fixture.session.sessionTabBySessionId(fixture.sessionId).click({ button: "right" });
    await testPage.getByRole("menuitem", { name: "Restart session", exact: true }).click();
    await expect.poll(() => gate.heldCount()).toBe(1);
    await expect
      .poll(() => testPage.evaluate("window.__KANDEV_E2E_STORE__.getState().tasks.activeSessionId"))
      .toBe(fixture.sessionId);
    await assertRestartSubmissionBlocked(testPage, fixture, draftText);
    await waitForFiniteAnimations(testPage.locator("body"));
    await prCapture.screenshot("desktop-restarting-composer", {
      caption: "Keep editing the draft while Restart session temporarily blocks Send.",
    });
    const launched = fixture.wire.waitForResponse("session.launch", { timeout: 30_000 });
    gate.release();
    gate.hold((request) => request.action === "session.launch");
    await expect.poll(() => gate.heldCount()).toBe(1);
    await assertRestartSubmissionBlocked(testPage, fixture, draftText);
    gate.release();
    await launched;
    await assertRestartRetained(apiClient, fixture.task.id, fixture.sessionId, before);
    await expect(testPage.getByTestId("session-restart-status")).not.toBeVisible();
    const send = fixture.session.activeChat().getByTestId("submit-message-button");
    await expect(send).toBeEnabled();
    const submitted = fixture.wire.waitForResponse("message.add");
    await send.click();
    await submitted;
    expect(
      fixture.traffic.frames.filter(
        (frame) => frame.direction === "sent" && frame.action === "message.add",
      ),
    ).toHaveLength(1);
  } finally {
    gate.release();
  }
});

// @covers AC-TASKS-SESSION-TAB-RESTART-001.1, AC-TASKS-SESSION-TAB-RESTART-001.2, AC-TASKS-SESSION-TAB-RESTART-001.3, AC-TASKS-SESSION-TAB-RESTART-001.5
test("keeps history and recovery available after restarting an interrupted turn", async ({
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
  const launch = fixture.wire
    .waitForResponse("session.launch", { timeout: 30_000, timeoutAfter: stop })
    .catch((error: unknown) => error);
  const wireStart = fixture.traffic.frames.length;
  await fixture.session.sessionTabBySessionId(fixture.sessionId).click({ button: "right" });
  await testPage.getByRole("menuitem", { name: "Restart session", exact: true }).click();
  await stop;
  const launchOutcome = await launch;
  if (launchOutcome instanceof Error) {
    expect(launchOutcome.message).toContain("unknown_prompt_outcome");
  }
  await expect
    .poll(async () =>
      (await apiClient.getTaskSession(fixture.sessionId)).session.session_recovery_blocks?.some(
        (block) => block.reason === "unknown_prompt_outcome",
      ),
    )
    .toBe(true);
  const recovery = testPage.getByTestId("failed-session-banner");
  await expect(recovery).toContainText(
    "Kandev cannot confirm whether the agent processed your message.",
  );
  await expect(recovery.getByTestId("recovery-resume-button")).toBeEnabled();
  await assertRestartDataRetained(apiClient, fixture.task.id, fixture.sessionId, before);
  await expect(
    fixture.session.chat.getByText("simple mock response", { exact: false }),
  ).toBeVisible();
  // Focus signals are independent from the explicit Stop/Resume sequence.
  const requests = fixture.traffic.frames
    .slice(wireStart)
    .filter(
      (f) =>
        f.direction === "sent" &&
        f.activationSource !== "session_focus" &&
        ["session.stop", "session.launch"].includes(f.action ?? ""),
    );
  expect(requests.map((f) => [f.action, f.sessionId])).toEqual([
    ["session.stop", fixture.sessionId],
    ["session.launch", fixture.sessionId],
  ]);
});

for (const restartSelected of [false, true]) {
  test(`restarts ${restartSelected ? "the selected" : "a non-active"} tab with a live sibling and preserves the draft`, async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
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
    const targetId = restartSelected ? sibling.id : fixture.sessionId;
    const before = await captureRestartBaseline(apiClient, fixture.task.id, targetId);
    await fixture.session.sessionTabBySessionId(targetId).click({ button: "right" });
    const action = testPage.getByRole("menuitem", { name: "Restart session", exact: true });
    await expect(action).toBeVisible();
    if (restartSelected) {
      await waitForFiniteAnimations(testPage.getByRole("menu"));
      await prCapture.screenshot("desktop-session-restart-menu", {
        caption: "Restart the selected session while a live sibling remains available.",
      });
    }
    const stop = fixture.wire.waitForResponse("session.stop", {
      requestWhere: (payload) => payload.session_id === targetId,
    });
    const launch = fixture.wire.waitForResponse("session.launch", {
      timeout: 30_000,
      timeoutAfter: stop,
      requestWhere: (payload) => payload.session_id === targetId && payload.intent === "resume",
    });
    await action.click();
    await Promise.all([stop, launch]);
    await assertRestartRetained(apiClient, fixture.task.id, targetId, before);
    await expect(editor).toContainText("Keep this unsent draft");
    await expect
      .poll(() => testPage.evaluate("window.__KANDEV_E2E_STORE__.getState().tasks.activeSessionId"))
      .toBe(sibling.id);
  });
}

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
    const launch = fixture.wire.waitForResponse("session.launch", {
      timeout: 30_000,
      timeoutAfter: stop,
    });
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

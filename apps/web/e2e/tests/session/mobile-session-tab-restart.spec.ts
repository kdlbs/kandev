import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { routeNavigationResponses } from "../../helpers/navigation-response-hold";
import {
  createRestartScenario,
  captureRestartBaseline,
  assertRestartRetained,
  assertRestartSubmissionBlocked,
  routeRestartContinuationGuidance,
} from "./session-tab-restart-helpers";

// @covers AC-TASKS-SESSION-TAB-RESTART-001.6
test("mobile: blocks Send during restart and preserves an editable draft", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
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
    )
    .toBe(2);
  await testPage.getByTestId("mobile-sessions-pill").tap();
  await testPage.getByTestId(`mobile-session-row-${fixture.sessionId}`).tap();
  await expect(testPage.getByRole("dialog", { name: "Sessions", exact: true })).not.toBeVisible();
  const before = await captureRestartBaseline(apiClient, fixture.task.id, fixture.sessionId);
  const draftText = "Keep this phone draft during restart";
  await fixture.session.fillMessage(draftText);
  gate.hold((request) => request.action === "session.stop");
  try {
    await testPage.getByTestId("mobile-sessions-pill").tap();
    const sheet = testPage.getByRole("dialog", { name: "Sessions", exact: true });
    const row = sheet.getByTestId(`mobile-session-row-${fixture.sessionId}`);
    await row.getByRole("button", { name: "Session actions" }).click();
    const action = testPage.getByRole("menuitem", { name: "Restart session", exact: true });
    await expect(action).toBeVisible();
    await waitForFiniteAnimations(testPage.getByRole("menu"));
    await action.tap();
    await expect.poll(() => gate.heldCount()).toBe(1);
    await testPage
      .locator('[data-slot="drawer-overlay"][data-state="open"]')
      .tap({ position: { x: 16, y: 16 } });
    await expect(sheet).not.toBeVisible();
    await assertRestartSubmissionBlocked(testPage, fixture, draftText);
    await waitForFiniteAnimations(testPage.locator("body"));
    await prCapture.screenshot("phone-restarting-composer", {
      caption: "The phone composer keeps its draft editable while Send is blocked during restart.",
    });
    await assertNoDocumentHorizontalOverflow(testPage, "phone restarting composer");
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
    await send.tap();
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

// @covers AC-TASKS-SESSION-TAB-RESTART-001.1, AC-TASKS-SESSION-TAB-RESTART-001.2, AC-TASKS-SESSION-TAB-RESTART-001.5
test("mobile: restarts from the session actions sheet and retains the conversation", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
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
    )
    .toBe(2);
  await testPage.getByTestId("mobile-sessions-pill").tap();
  await testPage.getByTestId(`mobile-session-row-${fixture.sessionId}`).tap();
  await expect(testPage.getByRole("dialog", { name: "Sessions", exact: true })).not.toBeVisible();
  const draft = await fixture.session.fillMessage("Keep this phone draft");
  const before = await captureRestartBaseline(apiClient, fixture.task.id, fixture.sessionId);
  await testPage.getByTestId("mobile-sessions-pill").tap();
  const sheet = testPage.getByRole("dialog", { name: "Sessions", exact: true });
  const row = sheet.getByTestId(`mobile-session-row-${fixture.sessionId}`);
  const trigger = row.getByRole("button", { name: "Session actions" });
  expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await trigger.click();
  const action = testPage.getByRole("menuitem", { name: "Restart session", exact: true });
  await expect(action).toBeVisible();
  await waitForFiniteAnimations(testPage.getByRole("menu"));
  const box = (await action.boundingBox())!;
  expect(box.height).toBeGreaterThanOrEqual(44);
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile restart menu");
  await prCapture.screenshot("phone-session-restart-menu", {
    caption: "Restart the selected conversation from the phone session menu.",
  });
  const stop = fixture.wire.waitForResponse("session.stop");
  const launch = fixture.wire.waitForResponse("session.launch", { timeoutAfter: stop });
  await action.tap();
  await Promise.all([stop, launch]);
  await assertRestartRetained(apiClient, fixture.task.id, fixture.sessionId, before);
  await expect(trigger).toBeFocused();
  await row.tap();
  await expect(sheet).not.toBeVisible();
  await expect(draft).toContainText("Keep this phone draft");
  await expect(
    fixture.session.chat.getByText("simple mock response", { exact: false }),
  ).toBeVisible();
  await testPage.reload();
  await fixture.session.waitForLoad();
  await expect(
    fixture.session.chat.getByText("simple mock response", { exact: false }),
  ).toBeVisible();
});

test("mobile: keeps typed continuation guidance after Restart closes its menus", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 320, height: 900 });
  let routed: Awaited<ReturnType<typeof routeRestartContinuationGuidance>> | undefined;
  const fixture = await createRestartScenario(testPage, apiClient, seedData, async (sessionId) => {
    routed = await routeRestartContinuationGuidance(testPage, sessionId);
  });
  const stop = fixture.wire.waitForResponse("session.stop", {
    requestWhere: (payload) => payload.session_id === fixture.sessionId,
  });
  const launch = fixture.wire
    .waitForResponse("session.launch", {
      requestWhere: (payload) => payload.session_id === fixture.sessionId,
      timeoutAfter: stop,
    })
    .catch((error: unknown) => error);
  await testPage.getByTestId("mobile-sessions-pill").tap();
  const sheet = testPage.getByRole("dialog", { name: "Sessions", exact: true });
  const row = sheet.getByTestId(`mobile-session-row-${fixture.sessionId}`);
  await row.getByRole("button", { name: "Session actions" }).tap();
  const menu = testPage.getByRole("menu");
  await testPage.getByRole("menuitem", { name: "Restart session", exact: true }).tap();
  await stop;
  expect(await launch).toBeInstanceOf(Error);
  await expect(menu).not.toBeVisible();
  await testPage.locator('[data-slot="drawer-overlay"][data-state="open"]').tap({
    position: { x: 16, y: 16 },
  });
  await expect(sheet).not.toBeVisible();

  const card = testPage.getByTestId("session-recovery-card");
  const continuation = card.getByTestId("recovery-continue-from-history-button");
  await expect(continuation).toBeVisible();
  const buttonBox = await continuation.boundingBox();
  expect(buttonBox?.height).toBeGreaterThanOrEqual(44);
  expect(buttonBox?.x).toBeGreaterThanOrEqual(0);
  expect(buttonBox!.x + buttonBox!.width).toBeLessThanOrEqual(320);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile Restart recovery card");
  expect(routed?.requests.filter((request) => request.action === "session.recover")).toEqual([]);

  const retry = fixture.wire
    .waitForResponse("session.recover", {
      requestWhere: (payload) => payload.action === "continue_from_history",
    })
    .catch((error: unknown) => error);
  await continuation.tap();
  expect(await retry).toBeInstanceOf(Error);
  await expect(continuation).toBeVisible();
  await expect(continuation).toBeEnabled();
  const failure = card.getByTestId("session-recovery-error");
  await expect(failure).toBeVisible();
  await expect(failure).toHaveText("Failed to resume session");
  expect(routed?.requests.filter((request) => request.action === "session.recover")).toHaveLength(
    1,
  );
});

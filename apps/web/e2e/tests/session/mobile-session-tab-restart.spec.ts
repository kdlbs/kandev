import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  createRestartScenario,
  captureRestartBaseline,
  assertRestartRetained,
} from "./session-tab-restart-helpers";

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

import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import {
  isBackgroundUpdateNotice,
  observeSessionSelection,
  readSelectionToasts,
  unexpectedSelectionRequests,
} from "../../helpers/session-selection-observation";
import { SessionPage } from "../../pages/session-page";

// Native phone controls expose selection and labels, but no session rename action.
// @covers AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.4
test("mobile: canonical Unicode session names survive native picker selection", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Unicode session picker",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing session ID");
  const secondary = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
    sessionId: `unicode-${task.id}`,
    startedAt: "2026-01-01T00:01:00Z",
  });
  const initial = "🧭".repeat(120);
  const mixed = `${"漢".repeat(119)}🧭`;
  await apiClient.renameSession(task.session_id, initial);
  await apiClient.renameSession(secondary.session_id, mixed);
  const traffic = attachGatewayTrafficCapture(testPage);
  const lifecycle = observeSessionSelection(testPage);
  await testPage.goto(`/t/${task.id}`);
  await new SessionPage(testPage).waitForLoad();
  const pill = testPage.getByTestId("mobile-task-layout").getByTestId("mobile-sessions-pill");
  await expect(pill).toContainText(initial);
  await pill.tap();
  const primaryRow = testPage.getByTestId(`mobile-session-row-${task.session_id}`);
  const secondaryRow = testPage.getByTestId(`mobile-session-row-${secondary.session_id}`);
  await expect(primaryRow).toContainText(initial);
  await expect(secondaryRow).toContainText(mixed);
  await secondaryRow.tap();
  await expect(pill).toContainText(mixed);
  await pill.tap();
  await expect(secondaryRow).toHaveAttribute("aria-current", "true");
  await expect
    .poll(() =>
      traffic.frames.some(
        (frame) =>
          frame.direction === "sent" &&
          frame.action === "session.subscribe" &&
          frame.sessionId === secondary.session_id,
      ),
    )
    .toBe(true);
  await dwell(
    testPage,
    300,
    "negative-assertion",
    "Touch session selection has no lifecycle completion event; observe for an unintended action",
  );
  const toasts = await readSelectionToasts(testPage);
  await test.info().attach("session-selection-observation", {
    body: JSON.stringify({ taskId: task.id, liveSessionId: task.session_id, lifecycle, toasts }),
    contentType: "application/json",
  });
  expect(unexpectedSelectionRequests(lifecycle, task.id, task.session_id)).toEqual([]);
  expect(toasts.filter((toast) => !isBackgroundUpdateNotice(toast))).toEqual([]);
  await expect(testPage.getByTestId("session-tab-rename-input")).toHaveCount(0);
  await expect(testPage.getByTestId("preview-session-tab-rename-input")).toHaveCount(0);
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await waitForFiniteAnimations(testPage.getByRole("dialog", { name: "Sessions", exact: true }));
  await testPage.screenshot({ path: test.info().outputPath("unicode-session-phone.png") });
});

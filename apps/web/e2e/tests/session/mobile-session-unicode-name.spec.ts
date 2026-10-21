import { test, expect } from "../../fixtures/test-base";
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
  await secondaryRow.getByRole("button", { name: "Session actions" }).tap();
  const actions = testPage.getByRole("menu");
  await expect(actions).toBeVisible();
  await expect(actions.getByRole("menuitem", { name: "Rename", exact: true })).toHaveCount(0);
  await testPage.keyboard.press("Escape");
  await expect(actions).toHaveCount(0);
  await expect(testPage.getByTestId("session-tab-rename-input")).toHaveCount(0);
  await expect(testPage.getByTestId("preview-session-tab-rename-input")).toHaveCount(0);
  expect(
    await testPage.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  await testPage.screenshot({ path: test.info().outputPath("unicode-session-phone.png") });
});

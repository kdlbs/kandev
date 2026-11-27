import { test, expect } from "../../fixtures/test-base";

const SAVED_ACTIVE_TAB_KEY = "kandev.activeTab";
const CHAT_TAB = "chat";
const PR_TAB = "pr";

test("tablet falls back to Chat when a saved PR tab has no reviews", async ({
  tabletTestPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `Tablet saved PR fallback ${Date.now()}`,
    seedData.agentProfileId,
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      start_agent: false,
      prepare_session: true,
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await tabletTestPage.goto("/");
  const storageKey = `${SAVED_ACTIVE_TAB_KEY}.${task.session_id}`;
  await tabletTestPage.evaluate(
    ({ key, value }) => window.sessionStorage.setItem(key, JSON.stringify(value)),
    { key: storageKey, value: PR_TAB },
  );

  await tabletTestPage.goto(`/t/${task.id}`);
  await expect(tabletTestPage.getByTestId("tablet-task-layout")).toBeVisible();
  await expect(tabletTestPage.getByRole("tab", { name: "Chat", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect
    .poll(() => tabletTestPage.evaluate((key) => window.sessionStorage.getItem(key), storageKey))
    .toBe(JSON.stringify(CHAT_TAB));
});

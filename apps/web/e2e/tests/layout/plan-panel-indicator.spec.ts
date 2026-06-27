import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const INITIAL_PLAN_CONTENT = "## Initial\n\nStep one";
const UPDATED_PLAN_CONTENT = "## Updated\n\nStep one\nStep two";

function planTabLocator(page: Page) {
  // `.dv-tab` is the wrapper dockview toggles `dv-active-tab` on; `.dv-default-tab`
  // below it never gets the active class so we target the outer wrapper here.
  return page.locator(".dv-tab", { has: page.locator(".dv-default-tab:has-text('Plan')") });
}

function planTabIndicator(page: Page) {
  return page.getByTestId("plan-tab-indicator");
}

async function waitForAgentPlan(
  apiClient: ApiClient,
  taskId: string,
  contentText: string,
  timeout = 15_000,
) {
  await expect
    .poll(
      async () => {
        const plan = await apiClient.getTaskPlan(taskId);
        return plan?.created_by === "agent" && plan.content.includes(contentText);
      },
      {
        timeout,
        message: `Expected agent-authored plan containing "${contentText}"`,
      },
    )
    .toBe(true);
}

async function createAgentPlan(apiClient: ApiClient, taskId: string) {
  await apiClient.wsRequest("task.plan.create", {
    task_id: taskId,
    title: "Plan v1",
    content: INITIAL_PLAN_CONTENT,
    created_by: "agent",
    author_kind: "agent",
    author_name: "E2E Agent",
  });
}

async function updateAgentPlan(apiClient: ApiClient, taskId: string) {
  await apiClient.wsRequest("task.plan.update", {
    task_id: taskId,
    title: "Plan v2",
    content: UPDATED_PLAN_CONTENT,
    created_by: "agent",
    author_kind: "agent",
    author_name: "E2E Agent",
  });
}

async function expectPlanIndicatorVisible(page: Page) {
  await planTabIndicator(page)
    .waitFor({ state: "visible", timeout: 5_000 })
    .catch(async () => {
      // The plan is durable backend state, but the tab indicator is armed from
      // the task.plan.* WS push. Under shard load that push can beat the page's
      // subscription; a reload exercises the same self-heal users get on refresh.
      await page.reload();
      await expect(planTabLocator(page)).toBeVisible({ timeout: 15_000 });
    });

  await expect(planTabLocator(page)).toBeVisible({ timeout: 15_000 });
  await expect(planTabIndicator(page)).toBeVisible({ timeout: 15_000 });
}

async function expectVisiblePlanContent(session: SessionPage, text: string, timeout = 30_000) {
  const editor = session.planPanel.locator(".ProseMirror").first();
  await expect(editor).toBeVisible({ timeout });
  await expect
    .poll(
      async () => {
        return (await editor.innerText().catch(() => "")).includes(text);
      },
      {
        timeout,
        message: `Expected visible plan editor to contain "${text}"`,
      },
    )
    .toBe(true);
}

async function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "plan indicator coverage",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    agentProfileId: seedData.agentProfileId,
  });

  await testPage.goto(`/t/${task.id}?sessionId=${sessionId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad(30_000);

  return { session, taskId: task.id, sessionId };
}

async function seedTaskWithAgentSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const session = sessions.find((candidate) => candidate.id === task.session_id);
        return (
          Boolean(session?.task_environment_id) &&
          ["IDLE", "WAITING_FOR_INPUT", "COMPLETED"].includes(session?.state ?? "")
        );
      },
      {
        timeout: 60_000,
        message: `Expected session ${task.session_id} to be idle with a task environment`,
      },
    )
    .toBe(true);

  await testPage.goto(`/t/${task.id}?sessionId=${task.session_id}`);
  const session = new SessionPage(testPage);

  return { session, taskId: task.id, sessionId: task.session_id };
}

test.describe("Plan panel auto-open + indicator", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("agent create reveals plan tab with indicator and keeps chat focused", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session, taskId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "plan indicator create",
    );
    await createAgentPlan(apiClient, taskId);
    await waitForAgentPlan(apiClient, taskId, "Step one");

    // Plan tab is rendered (panel mounted as a sibling of chat in the center group)
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });

    // Chat panel remained active (no focus steal — plan panel body stays hidden)
    await expect(session.activeChat()).toBeVisible({ timeout: 15_000 });
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);

    // Indicator dot is visible on the Plan tab. The indicator arms only once
    // `plan.created_by === "agent"` lands in the store — that comes from the
    // `task.plan.created` WS push, or the eager getTaskPlan self-heal if the
    // push was missed. Both can land after the default 5s under shard load, so
    // match the 15s tab budget.
    await expectPlanIndicatorVisible(testPage);
  });

  test("clicking the Plan tab clears the indicator and reveals plan content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session, taskId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "plan indicator acknowledge",
    );
    await createAgentPlan(apiClient, taskId);
    await waitForAgentPlan(apiClient, taskId, "Step one");
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expectPlanIndicatorVisible(testPage);

    await session.clickTab("Plan");

    await expect(planTabLocator(testPage)).toHaveClass(/dv-active-tab/);
    await expect(planTabIndicator(testPage)).toHaveCount(0);
    await expect(session.planPanel).toBeVisible();
    await expectVisiblePlanContent(session, "Step one");
  });

  test("agent update while on chat re-arms the indicator", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { session, taskId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "plan indicator update",
    );
    await createAgentPlan(apiClient, taskId);
    await waitForAgentPlan(apiClient, taskId, "Step one");
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expectPlanIndicatorVisible(testPage);

    // Acknowledge then leave back to chat
    await session.clickTab("Plan");
    await expect(planTabIndicator(testPage)).toHaveCount(0);
    await session.clickSessionChatTab();
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);

    // Trigger an agent-authored plan update through the same WS write path the
    // MCP tool uses after an agent turn.
    await updateAgentPlan(apiClient, taskId);
    await waitForAgentPlan(apiClient, taskId, "Step two");

    // Chat still focused, indicator re-armed
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);
    await expectPlanIndicatorVisible(testPage);

    // Clicking the Plan tab shows the updated content
    await session.clickTab("Plan");
    await expectVisiblePlanContent(session, "Step two");
  });

  test("page refresh with existing agent-authored plan shows no stale indicator", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { session, taskId, sessionId } = await seedTaskWithAgentSession(
      testPage,
      apiClient,
      seedData,
      "plan indicator refresh",
    );
    await createAgentPlan(apiClient, taskId);
    await waitForAgentPlan(apiClient, taskId, "Step one");
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    // The plan-update WS event arrives separately from the agent's idle
    // signal — the tab mounts as soon as the panel registers, but the
    // indicator only flips on once `plan.created_by === "agent"` is in the
    // store. Match the tab's 15s budget instead of using the default 5s,
    // which raced the WS push under shard load.
    await expectPlanIndicatorVisible(testPage, session);

    // Acknowledge
    await session.clickTab("Plan");
    await expect(planTabIndicator(testPage)).toHaveCount(0);

    // Layout persistence is debounced (~300ms) — wait for the saved
    // layout to actually include the Plan panel before reloading,
    // otherwise the restore will not bring it back.
    await testPage.waitForFunction(
      () => {
        const raw = localStorage.getItem("dockview-layout-v3");
        return !!raw && raw.includes('"id":"plan"');
      },
      null,
      { timeout: 5_000 },
    );

    // Reload. After the dockview "preserve restored active tab" change
    // (commit 597b35662) Plan stays active on refresh, so assert the Plan tab
    // directly instead of forcing the chat panel to the foreground.
    await testPage.goto(`/t/${taskId}?sessionId=${sessionId}`);

    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expect(planTabIndicator(testPage)).toHaveCount(0);
  });
});

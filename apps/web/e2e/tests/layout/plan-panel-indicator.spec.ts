import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const CREATE_PLAN_SCRIPT = [
  'e2e:thinking("creating plan")',
  "e2e:delay(100)",
  'e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"## Initial\\n\\nStep one","title":"Plan v1"})',
  "e2e:delay(100)",
  'e2e:message("plan created")',
].join("\n");

const UPDATE_PLAN_SCRIPT = [
  'e2e:thinking("updating plan")',
  "e2e:delay(100)",
  'e2e:mcp:kandev:update_task_plan_kandev({"task_id":"{task_id}","content":"## Updated\\n\\nStep one\\nStep two"})',
  "e2e:delay(100)",
  'e2e:message("plan updated")',
].join("\n");

function planTabLocator(page: Page) {
  // `.dv-tab` is the wrapper dockview toggles `dv-active-tab` on; `.dv-default-tab`
  // below it never gets the active class so we target the outer wrapper here.
  return page.locator(".dv-tab", { has: page.locator(".dv-default-tab:has-text('Plan')") });
}

function planTabIndicator(page: Page) {
  return page.getByTestId("plan-tab-indicator");
}

async function waitForAgentPlan(apiClient: ApiClient, taskId: string, contentText: string) {
  await expect
    .poll(
      async () => {
        const plan = await apiClient.getTaskPlan(taskId);
        return plan?.created_by === "agent" && plan.content.includes(contentText);
      },
      {
        timeout: 15_000,
        message: `Expected agent-authored plan containing "${contentText}"`,
      },
    )
    .toBe(true);
}

async function expectPlanIndicatorVisible(page: Page, session: SessionPage) {
  await planTabIndicator(page)
    .waitFor({ state: "visible", timeout: 5_000 })
    .catch(async () => {
      // The plan is durable backend state, but the tab indicator is armed from
      // the task.plan.* WS push. Under shard load that push can beat the page's
      // subscription; a reload exercises the same self-heal users get on refresh.
      await page.reload();
      await session.waitForLoad();
    });

  await expect(planTabLocator(page)).toBeVisible({ timeout: 15_000 });
  await expect(planTabIndicator(page)).toBeVisible({ timeout: 15_000 });
}

async function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  return { session, taskId: task.id };
}

test.describe("Plan panel auto-open + indicator", () => {
  test.describe.configure({ retries: 1 });

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
      CREATE_PLAN_SCRIPT,
    );
    await waitForAgentPlan(apiClient, taskId, "Step one");

    // Plan tab is rendered (panel mounted as a sibling of chat in the center group)
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });

    // Chat panel remained active (no focus steal — plan panel body stays hidden)
    await expect(session.chat).toBeVisible();
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);

    // Indicator dot is visible on the Plan tab. The indicator arms only once
    // `plan.created_by === "agent"` lands in the store — that comes from the
    // `task.plan.created` WS push, or the eager getTaskPlan self-heal if the
    // push was missed. Both can land after the default 5s under shard load, so
    // match the 15s tab budget.
    await expectPlanIndicatorVisible(testPage, session);
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
      CREATE_PLAN_SCRIPT,
    );
    await waitForAgentPlan(apiClient, taskId, "Step one");
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expectPlanIndicatorVisible(testPage, session);

    await session.clickTab("Plan");

    await expect(planTabLocator(testPage)).toHaveClass(/dv-active-tab/);
    await expect(planTabIndicator(testPage)).toHaveCount(0);
    await expect(session.planPanel).toBeVisible();
    await expect(session.planPanel.getByText("Step one")).toBeVisible({ timeout: 10_000 });
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
      CREATE_PLAN_SCRIPT,
    );
    await waitForAgentPlan(apiClient, taskId, "Step one");
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expectPlanIndicatorVisible(testPage, session);

    // Acknowledge then leave back to chat
    await session.clickTab("Plan");
    await expect(planTabIndicator(testPage)).toHaveCount(0);
    await session.clickSessionChatTab();
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);

    // Trigger an agent update via a follow-up message
    await session.sendMessage(UPDATE_PLAN_SCRIPT);
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });
    await waitForAgentPlan(apiClient, taskId, "Step two");

    // Chat still focused, indicator re-armed
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);
    await expectPlanIndicatorVisible(testPage, session);

    // Clicking the Plan tab shows the updated content
    await session.clickTab("Plan");
    await expect(session.planPanel.getByText("Step two")).toBeVisible({ timeout: 15_000 });
  });

  test("page refresh with existing agent-authored plan shows no stale indicator", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { session, taskId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "plan indicator refresh",
      CREATE_PLAN_SCRIPT,
    );
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
    // (commit 597b35662) Plan stays active on refresh, so session-chat is
    // mounted but in the background — foreground it explicitly so the
    // page-loaded wait succeeds.
    await testPage.goto(`/t/${taskId}`);
    await session.showSessionContext();

    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expect(planTabIndicator(testPage)).toHaveCount(0);
  });
});

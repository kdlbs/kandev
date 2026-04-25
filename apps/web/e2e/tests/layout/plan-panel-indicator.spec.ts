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
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

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

    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "plan indicator create",
      CREATE_PLAN_SCRIPT,
    );

    // Plan tab is rendered (panel mounted as a sibling of chat in the center group)
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });

    // Chat panel remained active (no focus steal — plan panel body stays hidden)
    await expect(session.chat).toBeVisible();
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);

    // Indicator dot is visible on the Plan tab
    await expect(planTabIndicator(testPage)).toBeVisible();
  });

  test("clicking the Plan tab clears the indicator and reveals plan content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "plan indicator acknowledge",
      CREATE_PLAN_SCRIPT,
    );
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expect(planTabIndicator(testPage)).toBeVisible();

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

    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "plan indicator update",
      CREATE_PLAN_SCRIPT,
    );
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expect(planTabIndicator(testPage)).toBeVisible();

    // Acknowledge then leave back to chat
    await session.clickTab("Plan");
    await expect(planTabIndicator(testPage)).toHaveCount(0);
    await session.clickSessionChatTab();
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);

    // Trigger an agent update via a follow-up message
    await session.sendMessage(UPDATE_PLAN_SCRIPT);
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    // Chat still focused, indicator re-armed
    await expect(planTabLocator(testPage)).not.toHaveClass(/dv-active-tab/);
    await expect(planTabIndicator(testPage)).toBeVisible();

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
    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expect(planTabIndicator(testPage)).toBeVisible();

    // Acknowledge
    await session.clickTab("Plan");
    await expect(planTabIndicator(testPage)).toHaveCount(0);

    // Layout persistence is debounced (~300ms) — wait for the saved
    // layout to actually include the Plan panel before reloading,
    // otherwise the restore will not bring it back.
    await testPage.waitForFunction(
      () => {
        const raw = localStorage.getItem("dockview-layout-v1");
        return !!raw && raw.includes('"id":"plan"');
      },
      null,
      { timeout: 5_000 },
    );

    // Reload
    await testPage.goto(`/t/${taskId}`);
    await session.waitForLoad();

    await expect(planTabLocator(testPage)).toBeVisible({ timeout: 15_000 });
    await expect(planTabIndicator(testPage)).toHaveCount(0);
  });
});

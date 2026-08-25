// Filename starts with "mobile-" so this runs under the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

const PLAN_MARKER = "Mobile Plan table marker";
const WIDE_PLAN_MARKDOWN = [
  "## Mobile table",
  "",
  "| Status | Owner | Project | Branch | Review | Build | Deploy | Notes |",
  "| --- | --- | --- | --- | --- | --- | --- | --- |",
  `| Ready | Team Alpha | Kandev | main | Pending | Passing | Staged | ${PLAN_MARKER} |`,
].join("\n");

test.describe("mobile: Plan table column resizing", () => {
  test("keeps wide tables locally scrollable without resize controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Covers AC-UI-RESIZABLE-MARKDOWN-TABLES-001.12.
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Plan table",
      seedData.agentProfileId,
      {
        description: planScript(WIDE_PLAN_MARKDOWN),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
    await session.waitForChatIdle({ timeout: 45_000 });
    await session.composerReady();
    await session.togglePlanMode();
    await testPage.getByRole("button", { name: "Plan", exact: true }).tap();
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });

    const editor = session.planEditor();
    const table = editor.locator("table", { hasText: PLAN_MARKER });
    const wrapper = table.locator("xpath=..");
    await expect(table).toBeVisible({ timeout: 15_000 });
    await expect(editor.locator(".column-resize-handle")).toHaveCount(0);
    await expect(editor.locator(".resize-cursor")).toHaveCount(0);
    await expect(wrapper).toHaveClass(/tableWrapper/);
    expect(await wrapper.evaluate((element) => element.scrollWidth > element.clientWidth + 1)).toBe(
      true,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
  });
});

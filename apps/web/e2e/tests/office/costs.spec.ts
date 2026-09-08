import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Costs & Budgets", () => {
  test("cost summary starts at zero", async ({ officeApi, officeSeed }) => {
    const summary = await officeApi.getCostSummary(officeSeed.workspaceId);
    expect(summary).toBeDefined();
  });

  test("budgets list is initially empty", async ({ officeApi, officeSeed }) => {
    const budgets = await officeApi.listBudgets(officeSeed.workspaceId);
    expect(budgets).toBeDefined();
  });

  test("costs page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/costs");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Costs/i, {
      timeout: 10_000,
    });
  });

  test("costs page refetches the breakdown live when a task's project is reassigned", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const task = (await officeApi.createTask(officeSeed.workspaceId, "Costs Refetch Task", {
      workflow_id: officeSeed.workflowId,
    })) as Record<string, unknown>;
    const taskId = task.id as string;

    // Onboarding does not seed a default project, so a real project must be
    // created before it can be assigned to the task.
    const project = (await officeApi.createProject(
      officeSeed.workspaceId,
      "Costs Refetch Project",
    )) as Record<string, unknown>;
    const projectId = project.id as string;

    const breakdownPath = /\/costs\/breakdown$/;
    const initialLoad = waitForHttp(testPage, "GET", breakdownPath);
    await testPage.goto("/office/workspace/costs");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Costs/i, { timeout: 10_000 });
    await initialLoad;

    // Reassigning the task's project publishes office.task.updated with
    // fields: ["project_id"] (DashboardService.UpdateTaskProjectID). The
    // costs page's WS handler must gate a "costs" refetch trigger on that,
    // so an already-open tab sees the new breakdown without a manual
    // reload — this is the live-UI staleness gap from PR #2903's follow-up.
    // Assert the actual network refetch fires, not just that the WS frame
    // arrived, so a regression in the store trigger -> useOfficeRefetch wiring
    // fails this test too.
    const refetch = waitForHttp(testPage, "GET", breakdownPath);
    await officeApi.updateTaskProjectId(taskId, projectId);
    await refetch;
  });
});

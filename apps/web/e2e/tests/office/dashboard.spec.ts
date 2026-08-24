import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test.describe("Dashboard", () => {
  test("dashboard shows agent count", async ({ officeApi, officeSeed }) => {
    const dash = await officeApi.getDashboard(officeSeed.workspaceId);
    // At least 1 agent (CEO from onboarding); other tests may create additional agents.
    expect(dash.agent_count).toBeGreaterThanOrEqual(1);
    expect(dash.month_spend_subcents).toBe(0);
  });

  test("activity feed is defined", async ({ officeApi, officeSeed }) => {
    const activity = await officeApi.listActivity(officeSeed.workspaceId);
    expect(activity).toBeDefined();
  });

  test("dashboard API returns run_activity and task_breakdown", async ({
    officeApi,
    officeSeed,
  }) => {
    const dash = await officeApi.getDashboard(officeSeed.workspaceId);
    expect(dash.run_activity).toBeDefined();
    expect(Array.isArray(dash.run_activity)).toBe(true);
    expect(dash.run_activity.length).toBe(14);
    expect(dash.task_breakdown).toBeDefined();
    expect(dash.task_breakdown.open).toBeGreaterThanOrEqual(0);
    expect(dash.recent_tasks).toBeDefined();
    expect(Array.isArray(dash.recent_tasks)).toBe(true);
  });

  test("dashboard page renders all sections", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Tasks In Progress")).toBeVisible();
    await expect(testPage.getByText("Month Spend")).toBeVisible();
    await expect(testPage.getByText("Pending Approvals")).toBeVisible();
    await expect(testPage.getByText("Run Activity")).toBeVisible();
    await expect(testPage.getByText("Success Rate")).toBeVisible();
    await expect(testPage.getByRole("heading", { name: "Recent Activity" })).toBeVisible();
    await expect(testPage.getByRole("heading", { name: "Recent Tasks" })).toBeVisible();
  });

  test("stat cards link to detail pages", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    // Verify links exist by checking href attributes on ancestors of stat text
    const agentsCard = testPage.getByText("Agents Enabled").locator("xpath=ancestor::a");
    await expect(agentsCard).toHaveAttribute("href", "/office/agents");
    const tasksCard = testPage.getByText("Tasks In Progress").locator("xpath=ancestor::a");
    await expect(tasksCard).toHaveAttribute("href", "/office/tasks");
  });

  test("org chart page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/org");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Org/i, {
      timeout: 10_000,
    });
  });

  test("preferences page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/settings");
    await expect(officeTopbarTitle(testPage)).toHaveText(/preferences/i, {
      timeout: 10_000,
    });
  });

  test("update workspace settings", async ({ officeApi, officeSeed }) => {
    const { settings: originalSettings } = await officeApi.getWorkspaceSettings(
      officeSeed.workspaceId,
    );
    const originalLookbackHours = (originalSettings as { recovery_lookback_hours: number })
      .recovery_lookback_hours;

    try {
      const updated = await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
        recovery_lookback_hours: 42,
      });
      expect(updated).toEqual({ ok: true });

      const { settings } = await officeApi.getWorkspaceSettings(officeSeed.workspaceId);
      expect((settings as { recovery_lookback_hours: number }).recovery_lookback_hours).toBe(42);
    } finally {
      await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
        recovery_lookback_hours: originalLookbackHours,
      });
    }
  });
});

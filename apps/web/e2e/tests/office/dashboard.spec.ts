import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test("dashboard renders its metrics, activity and navigation after onboarding", async ({
  testPage,
  officeApi,
  officeSeed,
}) => {
  const dash = (await officeApi.getDashboard(officeSeed.workspaceId)) as {
    agent_count: number;
    month_spend_subcents: number;
    run_activity: unknown[];
    task_breakdown: { open: number };
    recent_tasks: unknown[];
  };
  expect(dash.agent_count).toBeGreaterThanOrEqual(1);
  expect(dash.month_spend_subcents).toBe(0);
  expect(Array.isArray(dash.run_activity)).toBe(true);
  expect(dash.run_activity).toHaveLength(14);
  expect(dash.task_breakdown.open).toBeGreaterThanOrEqual(0);
  expect(Array.isArray(dash.recent_tasks)).toBe(true);

  await testPage.goto("/office");
  await expect(testPage).toHaveURL(/\/office$/, { timeout: 10_000 });
  await expect(officeTopbarTitle(testPage)).toHaveText(/Dashboard/i);
  for (const label of [
    "Agents Enabled",
    "Tasks In Progress",
    "Month Spend",
    "Pending Approvals",
    "Run Activity",
    "Success Rate",
  ]) {
    await expect(testPage.getByText(label, { exact: true })).toBeVisible();
  }
  await expect(testPage.getByRole("heading", { name: "Recent Activity" })).toBeVisible();
  await expect(testPage.getByRole("heading", { name: "Recent Tasks" })).toBeVisible();
  await expect(testPage.getByText("Agents Enabled").locator("xpath=ancestor::a")).toHaveAttribute(
    "href",
    "/office/agents",
  );
  await expect(
    testPage.getByText("Tasks In Progress").locator("xpath=ancestor::a"),
  ).toHaveAttribute("href", "/office/tasks");
});

import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

test("mobile Office task creation only shows supported assignee controls", async ({
  testPage,
  officeApi,
  officeSeed,
}) => {
  const runId = Date.now();
  const projectName = `Mobile New Task Dialog Project ${runId}`;
  const taskTitle = `Mobile New Task Dialog Assignment ${runId}`;
  const project = (await officeApi.createProject(officeSeed.workspaceId, projectName)) as {
    id: string;
    name: string;
  };
  expect(project.id).toBeTruthy();

  await testPage.goto("/office/tasks");
  await testPage.locator('button:has(svg.tabler-icon-plus):has-text("New Task")').tap();

  const dialog = testPage.getByTestId("office-new-issue-dialog");
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  await dialog.locator("button:has(svg.tabler-icon-dots-vertical)").first().tap();
  await expect(dialog.getByRole("menuitem", { name: /reviewer|approver/i })).toHaveCount(0);

  await dialog.getByPlaceholder("Task title").fill(taskTitle);
  await dialog.getByRole("button", { name: "Project" }).tap();
  await testPage.getByRole("button", { name: project.name, exact: true }).tap();
  await dialog.getByRole("button", { name: "Assignee" }).tap();
  await testPage.getByRole("button", { name: "CEO", exact: true }).tap();

  const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
  await dialog.getByTestId("new-task-create-button").tap();
  const response = await created;
  const body = (await response.json()) as { id?: string };
  expect(body.id).toBeTruthy();

  await expect(testPage.locator("[data-sonner-toast]")).toContainText(/task created/i, {
    timeout: 10_000,
  });
  const stored = (await officeApi.getTask(body.id as string)) as {
    task: { assigneeAgentProfileId?: string; projectId?: string };
  };
  expect(stored.task.assigneeAgentProfileId).toBe(officeSeed.agentId);
  expect(stored.task.projectId).toBe(project.id);
});

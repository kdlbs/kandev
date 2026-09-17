import { test, expect } from "../../fixtures/test-base";
import { resetWorkspaceOrchestrators } from "../../helpers/orchestration";

test("coordinator overview joins canonical tasks and persistent chat without starting work", async ({
  testPage: page,
  backend,
  apiClient,
  seedData,
  prCapture,
}) => {
  await backend.restart({
    KANDEV_FEATURES_ORCHESTRATION: "true",
    KANDEV_FEATURES_PERSONAL_ASSISTANT: "false",
    KANDEV_FEATURES_OFFICE: "false",
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  const ws = seedData.workspaceId;
  await resetWorkspaceOrchestrators(page.request, backend.baseUrl, ws);
  const queued = await apiClient.createTask(ws, "Write a sample setup guide", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const done = await apiClient.createTask(ws, "Check the example links", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.updateTaskState(done.id, "COMPLETED");
  const input = await apiClient.seedTask(ws, "Choose an example heading", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const inputSession = await apiClient.seedTaskSession(input.task_id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
  });
  const review = await apiClient.createTask(ws, "Review the sample checklist", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.updateTaskState(review.id, "REVIEW");
  await page.goto(`/settings/workspaces/${ws}/orchestration/new`);
  await page.getByTestId("orchestrator-profile").click();
  await page.getByRole("option").first().click();
  await page.getByTestId("persona-executor-profile").click();
  await page.getByRole("option").filter({ hasNotText: "Inherit" }).first().click();
  await page.getByRole("button", { name: "Add orchestrator", exact: true }).click();
  await expect(page).not.toHaveURL(/\/new$/);
  const chief = new URL(page.url()).pathname.split("/").pop()!;
  await page.goto(`/workspaces/${ws}/coordinator?orchestratorId=${chief}`);
  await expect(page.getByTestId("coordinator-page")).toBeVisible();
  await expect(
    page
      .getByTestId("coordinator-group-queued")
      .getByRole("link", { name: "Write a sample setup guide" }),
  ).toBeVisible();
  await expect(
    page
      .getByTestId("coordinator-group-done")
      .getByRole("link", { name: "Check the example links" }),
  ).toBeVisible();
  const chat = page.getByTestId("orchestrator-conversation");
  await expect(chat).toBeVisible();
  await apiClient.seedSessionMessage(inputSession.session_id, {
    type: "clarification_request",
    content: "Should the example use a short or detailed heading?",
  });
  await expect(
    page.getByTestId("coordinator-group-input").getByText("Choose an example heading"),
  ).toBeVisible();
  await prCapture.screenshot("desktop-tasks-and-chat", {
    caption:
      "Canonical task groups beside the selected coordinator conversation. All data is synthetic.",
  });
  await prCapture.startRecording("coordinator-generic-walkthrough");
  await chat
    .locator("textarea")
    .pressSequentially("Summarize the example tasks.", { delay: prCapture.capturing ? 35 : 0 });
  await page.getByLabel("Search tasks", { exact: true }).fill("links");
  await expect(page.getByTestId(`coordinator-task-${queued.id}`)).toHaveCount(0);
  await expect(chat.locator("textarea")).toHaveValue("Summarize the example tasks.");
  await page.getByLabel("Search tasks", { exact: true }).clear();
  await expect(page.getByTestId(`coordinator-task-${queued.id}`)).toBeVisible();
  await prCapture.stopRecording({
    caption: "Type a generic draft with the workspace tasks visible.",
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("tab", { name: "Chat", exact: true }).click();
  await expect(chat.locator("textarea")).toHaveValue("Summarize the example tasks.");
  await page.getByRole("tab", { name: "Tasks", exact: true }).click();
  await expect(page.getByTestId(`coordinator-task-${queued.id}`)).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  const conversation = await (
    await page.request.post(
      `${backend.baseUrl}/api/v1/orchestration/workspaces/${ws}/orchestrators/${chief}/conversation`,
    )
  ).json();
  const comments = await (
    await page.request.get(
      `${backend.baseUrl}/api/v1/orchestration/tasks/${conversation.task_id}/comments`,
    )
  ).json();
  expect(comments.comments).toEqual([]);
  const sessions = await apiClient.listTaskSessions(conversation.task_id);
  expect(sessions.sessions).toEqual([]);
  await page.getByRole("tab", { name: "Chat", exact: true }).click();
  await expect(chat.locator("textarea")).toHaveValue("Summarize the example tasks.");
  await page.setViewportSize({ width: 1440, height: 1000 });
  const base = `${backend.baseUrl}/api/v1/orchestration`;
  const first = await (
    await page.request.get(`${base}/workspaces/${ws}/orchestrators/${chief}`)
  ).json();
  const roleResponse = await page.request.post(`${base}/roles`, {
    data: { name: "Checklist coordinator", instructions: "Help with example checklists." },
  });
  expect(roleResponse.ok()).toBeTruthy();
  const role = await roleResponse.json();
  const secondResponse = await page.request.post(`${base}/workspaces/${ws}/orchestrators`, {
    data: { ...first, role_id: role.id },
  });
  expect(secondResponse.ok()).toBeTruthy();
  const second = await secondResponse.json();
  await apiClient.updateTaskMetadata(queued.id, { orchestration_chief_id: chief });
  await page.reload();
  await expect(chat.locator("textarea")).toBeVisible();
  await chat.locator("textarea").fill("Compare the sample guides.");
  await page.getByTestId("coordinator-selector").click();
  await page.getByRole("option", { name: "Checklist coordinator", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`orchestratorId=${second.id}`));
  await expect(chat.locator("textarea")).toHaveValue("");
  await chat.locator("textarea").fill("Check the sample checklist.");
  await page.getByTestId("coordinator-selector").click();
  await page.getByRole("option", { name: first.name, exact: true }).click();
  await expect(chat.locator("textarea")).toHaveValue("Compare the sample guides.");
  await page.getByRole("combobox", { name: "Task scope", exact: true }).click();
  await page.getByRole("option", { name: "Selected coordinator’s tasks", exact: true }).click();
  await expect(page.getByTestId(`coordinator-task-${queued.id}`)).toBeVisible();
  await expect(page.getByTestId(`coordinator-task-${done.id}`)).toHaveCount(0);
  await prCapture.screenshot("selected-coordinator", {
    caption: "Tasks filtered to one coordinator while keeping its own draft.",
  });
  await chat.locator("textarea").press("Enter");
  await expect
    .poll(
      async () => {
        const response = await (
          await page.request.get(
            `${backend.baseUrl}/api/v1/orchestration/tasks/${conversation.task_id}/comments`,
          )
        ).json();
        return response.comments.some(
          (comment: { source: string }) => comment.source === "session",
        );
      },
      { timeout: 60000 },
    )
    .toBe(true);
  const other = await apiClient.createWorkspace("Example second workspace");
  try {
    await page.goto(`/workspaces/${other.id}/coordinator?orchestratorId=${chief}`);
    await expect(page.getByTestId("coordinator-page")).toBeVisible();
    await expect(page.getByTestId(`coordinator-task-${queued.id}`)).toHaveCount(0);
    await expect(chat).toHaveCount(0);
    await expect(page.getByRole("alert")).toContainText("unavailable");
  } finally {
    await apiClient.deleteWorkspace(other.id, other.name);
  }
});

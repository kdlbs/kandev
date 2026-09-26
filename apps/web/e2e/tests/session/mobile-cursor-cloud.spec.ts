import { expect, test } from "../../fixtures/cursor-cloud";
import {
  createCursorCloudTask,
  cursorCloudGitHubURL,
  disableCursorCloudExecutors,
  findCursorCloudSessionId,
  getCursorCloudSessionStatus,
} from "./cursor-cloud-helpers";

test.afterEach(async ({ apiClient }) => {
  await disableCursorCloudExecutors(apiClient);
});

test("shows cloud task controls and results in phone surfaces", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  const { task, executorProfile } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud phone task",
  );

  await expect.poll(() => cursorCloud.count("POST", "/v1/agents")).toBe(1);
  await testPage.goto(`/settings/executors/${executorProfile.id}`);
  const testConnection = testPage.getByRole("button", { name: "Test connection" });
  await testConnection.scrollIntoViewIfNeeded();
  await expect(testConnection).toHaveCSS("min-height", "44px");
  const buttonBox = await testConnection.boundingBox();
  expect(buttonBox?.height).toBeGreaterThanOrEqual(44);
  const centerHitsButton = await testConnection.evaluate((button) => {
    const box = button.getBoundingClientRect();
    const target = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
    return Boolean(target && button.contains(target));
  });
  expect(centerHitsButton).toBe(true);
  await testConnection.click();
  await expect(testPage.getByText("Mock Cursor Model")).toBeVisible();

  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toBeVisible();
  await expect(testPage.getByRole("button", { name: "Results" })).toBeVisible();
  await testPage.getByRole("button", { name: "Results" }).click();
  await expect(testPage.getByRole("heading", { name: "Remote results" })).toBeVisible();
  const pullRequest = testPage.getByRole("link", { name: "Open pull request" });
  await expect(pullRequest).toHaveAttribute("href", `${cursorCloudGitHubURL}/pull/42`);
  await expect(pullRequest).toHaveCSS("min-height", "44px");
  expect(cursorCloud.prompts).toHaveLength(1);
  expect(cursorCloud.prompts[0]).toContain("Implement the requested remote change");

  await apiClient.archiveTask(task.id);
  await expect(testPage.getByTestId("chat-input-editor")).toHaveCount(0);
});

test("uses workflow-generated prompts and freezes model controls on phone", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  const { task, executorProfile } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud phone workflow prompt",
    { workflowPrompt: "Workflow step instruction: verify managed dispatch." },
  );

  await expect.poll(() => cursorCloud.prompts.length).toBe(1);
  await expect
    .poll(() => findCursorCloudSessionId(apiClient, task.id, executorProfile.id))
    .not.toBe("");
  const sessionId = await findCursorCloudSessionId(apiClient, task.id, executorProfile.id);
  await expect
    .poll(() => getCursorCloudSessionStatus(apiClient, task.id, sessionId))
    .toMatchObject({ remote_state: "succeeded" });
  expect(cursorCloud.prompts[0]).toContain("Workflow step instruction: verify managed dispatch.");
  expect(cursorCloud.prompts[0]).toContain("Implement the requested remote change");

  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toBeVisible();
  await expect(testPage.getByTestId("toolbar-item-model")).toHaveCount(0);
  const modelChange = await testPage.request.post(
    `${backend.baseUrl}/api/v1/task-sessions/${sessionId}/set-model`,
    { data: { model_id: "changed-after-start" } },
  );
  expect(modelChange.status()).toBe(404);
  expect(cursorCloud.count("POST", "/v1/agents")).toBe(1);
});

test("cancels a running remote agent when its task is archived on phone", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
}) => {
  cursorCloud.reset();
  cursorCloud.holdNextStreamOpen();
  const { task } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud phone running archive",
  );

  await expect.poll(() => cursorCloud.streamRequestCount()).toBeGreaterThan(0);
  await apiClient.archiveTask(task.id);
  await expect
    .poll(
      () =>
        cursorCloud.requests.filter(
          (request) => request.method === "POST" && request.path.endsWith("/cancel"),
        ).length,
    )
    .toBe(1);

  cursorCloud.releaseCancellation();
  expect(cursorCloud.count("POST", "/v1/agents")).toBe(1);
});

test("keeps an uncertain phone follow-up unresolved when its task is archived", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  const { task } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud phone unknown archive",
  );
  await expect
    .poll(() => getCursorCloudSessionStatus(apiClient, task.id, task.session_id))
    .toMatchObject({ remote_state: "succeeded" });
  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toBeVisible();

  cursorCloud.failNextSubmission("unknown-followup");
  await testPage.getByTestId("chat-input-editor").fill("Archive an uncertain phone follow-up");
  await testPage.getByTestId("submit-message-button").tap();
  await expect(testPage.getByTestId("cursor-cloud-submission-unknown")).toBeVisible();
  expect(cursorCloud.prompts).toHaveLength(2);

  await apiClient.archiveTask(task.id);
  await expect(testPage.getByTestId("cursor-cloud-submission-unknown")).toBeVisible();
  await expect(testPage.getByTestId("chat-input-editor")).toHaveCount(0);
  const resolutionResponse = await testPage.request.get(
    `${backend.baseUrl}/api/v1/tasks/${task.id}/sessions/${task.session_id}/cursor-cloud/submission`,
  );
  expect(resolutionResponse.ok()).toBe(true);
  expect(await resolutionResponse.json()).toMatchObject({ state: "unknown" });
  expect(
    cursorCloud.requests.filter(
      (request) => request.method === "POST" && request.path.endsWith("/cancel"),
    ),
  ).toHaveLength(0);
  expect(cursorCloud.count("POST", "/v1/agents")).toBe(1);
});

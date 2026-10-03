import { expect, test } from "../../fixtures/cursor-cloud";
import {
  createCursorCloudTask,
  disableCursorCloudExecutors,
  getCursorCloudSessionStatus,
} from "./cursor-cloud-helpers";

test.afterEach(async ({ apiClient }) => {
  await disableCursorCloudExecutors(apiClient);
});

test("binds an accepted unknown follow-up without submitting it again", async ({
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
    "Cursor Cloud unknown follow-up",
  );
  await expect.poll(() => cursorCloud.prompts.length).toBe(1);
  await expect
    .poll(() => getCursorCloudSessionStatus(apiClient, task.id, task.session_id), {
      timeout: 15000,
      intervals: [500, 1000],
    })
    .toMatchObject({ remote_state: "succeeded", state: "WAITING_FOR_INPUT" });
  const { sessions } = await apiClient.listTaskSessions(task.id);
  const cloudSession = sessions.find((session) => session.id === task.session_id);
  expect(cloudSession?.error_message ?? "").toBe("");
  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByText("Remote result is ready")).toBeVisible();
  const reopenedSessions = await apiClient.listTaskSessions(task.id);
  const reopenedCloudSession = reopenedSessions.sessions.find(
    (session) => session.id === task.session_id,
  );
  expect(reopenedCloudSession?.error_message ?? "").toBe("");

  cursorCloud.failNextSubmission("unknown-followup");
  const composer = testPage.getByTestId("chat-input-editor");
  await expect(composer).toHaveAttribute("contenteditable", "true");
  await composer.fill("Follow-up with an uncertain response");
  await testPage.getByTestId("submit-message-button").click();
  await expect.poll(() => cursorCloud.prompts.length).toBe(2);
  await expect(testPage.getByTestId("cursor-cloud-submission-unknown")).toBeVisible();

  await testPage.getByRole("button", { name: "Resolve submission" }).click();
  const candidate = testPage.getByRole("radio", { name: /run-2/ });
  await expect(candidate).toBeVisible();
  await candidate.check();
  await testPage.getByRole("button", { name: "Bind selected run" }).click();

  await expect(testPage.getByTestId("cursor-cloud-submission-unknown")).toBeHidden();
  await expect(testPage.getByText("Follow-up run-2 result is ready")).toBeVisible();
  expect(cursorCloud.prompts).toHaveLength(2);
  expect(cursorCloud.prompts[0]).toContain("Implement the requested remote change");
  expect(cursorCloud.prompts[1]).toContain("Follow-up with an uncertain response");
});

test("reconnects the same remote run after a backend restart", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  cursorCloud.holdNextStreamOpen();
  const { task } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud restart recovery",
  );
  await expect.poll(() => cursorCloud.streamRequestCount()).toBeGreaterThan(0);
  const initialStreams = cursorCloud.streamRequestCount();

  await backend.restart();
  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toBeVisible();
  await expect.poll(() => cursorCloud.streamRequestCount()).toBeGreaterThan(initialStreams);
  await expect(testPage.getByText("Remote result is ready")).toBeVisible();

  expect(cursorCloud.prompts).toHaveLength(1);
  expect(cursorCloud.prompts[0]).toContain("Implement the requested remote change");
  expect(cursorCloud.count("POST", "/v1/agents")).toBe(1);
});

test("shows a durable history-gap notice after provider stream retention expires", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  cursorCloud.failNextStream("retention-expired");
  const { task } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud history gap",
  );
  await expect.poll(() => cursorCloud.streamRequestCount()).toBeGreaterThanOrEqual(2);
  await expect
    .poll(() => getCursorCloudSessionStatus(apiClient, task.id, task.session_id), {
      timeout: 15000,
      intervals: [500, 1000],
    })
    .toMatchObject({ remote_history_gap: true });
  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toBeVisible();
  await expect(testPage.getByTestId("cursor-cloud-history-gap")).toBeVisible();
  await expect(testPage.getByText("Remote result is ready")).toBeVisible();
});

test("rejects an invalid callback grant before it can call a task tool", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  await createCursorCloudTask(apiClient, backend, seedData, "Cursor Cloud callback rejection");
  await expect.poll(() => cursorCloud.count("POST", "/v1/agents")).toBe(1);

  const response = await testPage.request.post(
    `${backend.baseUrl}/api/v1/managed-agent-mcp/invalid-grant`,
    {
      headers: { Authorization: "Bearer invalid" },
      data: { jsonrpc: "2.0", method: "tools/call", id: 1 },
    },
  );
  expect(response.status()).toBe(401);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toHaveCount(0);
  expect(cursorCloud.prompts).toHaveLength(1);
  expect(cursorCloud.prompts[0]).toContain("Implement the requested remote change");
});

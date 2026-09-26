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

test("starts a configured cloud task and displays remote results", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  const initialAgents = await apiClient.listAvailableAgents();
  expect(initialAgents.agents.some((agent) => agent.name === "cursor_cloud")).toBe(false);

  const { task, executorProfile } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud desktop task",
  );

  await expect.poll(() => cursorCloud.count("POST", "/v1/agents")).toBe(1);
  await expect
    .poll(() => getCursorCloudSessionStatus(apiClient, task.id, task.session_id), {
      timeout: 15000,
      intervals: [500, 1000],
    })
    .toMatchObject({ remote_state: "succeeded" });
  const remoteStatus = await getCursorCloudSessionStatus(apiClient, task.id, task.session_id);
  expect(remoteStatus.remote_branch).toBe("cursor/kandev-result");
  await testPage.goto(`/settings/executors/${executorProfile.id}`);
  await testPage.getByRole("button", { name: "Test connection" }).click();
  await expect(testPage.getByText("Mock Cursor Model")).toBeVisible();
  expect(cursorCloud.count("GET", "/v1/models")).toBeGreaterThan(0);
  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toBeVisible();
  await expect(testPage.getByText("Remote result is ready")).toBeVisible();
  await expect(testPage.getByText("cursor/kandev-result")).toBeVisible();
  await expect(testPage.getByRole("link", { name: "Open pull request" })).toHaveAttribute(
    "href",
    `${cursorCloudGitHubURL}/pull/42`,
  );
  expect(cursorCloud.prompts).toHaveLength(1);
  expect(cursorCloud.prompts[0]).toContain("Implement the requested remote change");

  await apiClient.archiveTask(task.id);
  await expect(testPage.getByTestId("chat-input-editor")).toHaveCount(0);
});

test("discovers the agent type only while a configured executor is available", async ({
  backend,
  cursorCloud,
  apiClient,
}) => {
  cursorCloud.reset();
  const absent = await apiClient.listAvailableAgents();
  expect(absent.agents.some((agent) => agent.name === "cursor_cloud")).toBe(false);

  const unsavedExecutor = await apiClient.createExecutor(
    "Cursor Cloud without a profile",
    "cursor_cloud",
  );
  expect(
    (await apiClient.listAvailableAgents()).agents.some((agent) => agent.name === "cursor_cloud"),
  ).toBe(false);
  await apiClient.deleteExecutor(unsavedExecutor.id);

  const firstSecret = await apiClient.createSecret(
    "Cursor Cloud discovery one",
    "cursor-cloud-key",
  );
  const firstExecutor = await apiClient.createExecutor("Cursor Cloud configured", "cursor_cloud");
  await apiClient.createExecutorProfile(firstExecutor.id, {
    name: "Configured profile",
    config: {
      cursor_cloud_api_key_secret_id: firstSecret.id,
      cursor_cloud_callback_url: `${backend.baseUrl}/api/v1/managed-agent-mcp`,
    },
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  let availableAgents = await apiClient.listAvailableAgents();
  expect(availableAgents.agents.some((agent) => agent.name === "cursor_cloud")).toBe(true);

  let agents = await apiClient.listAgents();
  const cloudAgent =
    agents.agents.find((agent) => agent.name === "cursor_cloud") ??
    (await apiClient.createAgent("cursor_cloud"));
  expect(cloudAgent).toBeDefined();
  const agentProfile = await apiClient.createAgentProfile(cloudAgent!.id, "Saved cloud agent", {
    model: "mock-cursor-model",
  });

  cursorCloud.expireCredentials();
  availableAgents = await apiClient.listAvailableAgents();
  expect(availableAgents.agents.some((agent) => agent.name === "cursor_cloud")).toBe(true);
  await apiClient.deleteExecutor(firstExecutor.id);
  availableAgents = await apiClient.listAvailableAgents();
  expect(availableAgents.agents.some((agent) => agent.name === "cursor_cloud")).toBe(false);
  agents = await apiClient.listAgents();
  const savedCloudAgent = agents.agents.find((agent) => agent.name === "cursor_cloud");
  expect(savedCloudAgent?.profiles.some((profile) => profile.id === agentProfile.id)).toBe(true);

  const secondSecret = await apiClient.createSecret(
    "Cursor Cloud discovery two",
    "cursor-cloud-key",
  );
  const secondExecutor = await apiClient.createExecutor("Cursor Cloud configured", "cursor_cloud");
  await apiClient.createExecutorProfile(secondExecutor.id, {
    name: "Configured profile",
    config: {
      cursor_cloud_api_key_secret_id: secondSecret.id,
      cursor_cloud_callback_url: `${backend.baseUrl}/api/v1/managed-agent-mcp`,
    },
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  availableAgents = await apiClient.listAvailableAgents();
  expect(availableAgents.agents.some((agent) => agent.name === "cursor_cloud")).toBe(true);
  agents = await apiClient.listAgents();
  const restoredCloudAgent = agents.agents.find((agent) => agent.name === "cursor_cloud");
  expect(restoredCloudAgent?.profiles?.some((profile) => profile.id === agentProfile.id)).toBe(
    true,
  );
});

test("uses workflow-generated prompts and freezes model controls", async ({
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
    "Cursor Cloud workflow prompt",
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

test("cancels a running remote agent when its task is archived", async ({
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
    "Cursor Cloud running archive",
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

test("keeps an uncertain follow-up unresolved when its task is archived", async ({
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
    "Cursor Cloud unknown archive",
  );
  await expect
    .poll(() => getCursorCloudSessionStatus(apiClient, task.id, task.session_id))
    .toMatchObject({ remote_state: "succeeded" });
  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toBeVisible();

  cursorCloud.failNextSubmission("unknown-followup");
  await testPage.getByTestId("chat-input-editor").fill("Archive an uncertain follow-up");
  await testPage.getByTestId("submit-message-button").click();
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

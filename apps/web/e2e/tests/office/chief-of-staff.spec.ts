import { test, expect } from "../../fixtures/office-fixture";

test("chief runs a configured routine through the durable scheduler", async ({
  testPage,
  officeApi,
  officeSeed,
  seedData,
  backend,
}) => {
  const chief = await officeApi.createAgent(officeSeed.workspaceId, {
    name: "Routine coordinator",
    role: "assistant",
    agent_profile_id: seedData.agentProfileId,
  });
  await officeApi.updateAgent(chief.id as string, {
    executor_preference: JSON.stringify({ type: "local_pc" }),
  });
  const response = await testPage.request.post(
    `${backend.baseUrl}/api/v1/office/workspaces/${officeSeed.workspaceId}/routines`,
    {
      data: {
        name: "Review blocked work",
        assignee_agent_profile_id: chief.id,
        task_template: JSON.stringify({
          title: "Review blocked work",
          description: "Give a concise status report.",
        }),
      },
    },
  );
  expect(response.ok()).toBeTruthy();
  const { routine } = await response.json();
  const fired = await testPage.request.post(
    `${backend.baseUrl}/api/v1/office/routines/${routine.id}/run`,
    { data: {} },
  );
  expect(fired.ok()).toBeTruthy();
  const { run } = await fired.json();
  expect(run.linked_task_id).toBeTruthy();
  await expect
    .poll(
      async () => {
        const res = await testPage.request.get(
          `${backend.baseUrl}/api/v1/office/tasks/${run.linked_task_id}/comments`,
        );
        const { comments } = await res.json();
        return comments.some((comment: { authorId: string }) => comment.authorId === chief.id);
      },
      { timeout: 30_000 },
    )
    .toBeTruthy();
});

test("native persona conversation reopens the same Office task", async ({
  testPage,
  officeSeed,
  officeApi,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  const chief = await officeApi.createAgent(officeSeed.workspaceId, {
    name: "Chief of staff",
    role: "assistant",
    agent_profile_id: seedData.agentProfileId,
  });
  const chiefId = chief.id as string;
  await officeApi.updateAgent(chiefId, {
    executor_preference: JSON.stringify({ type: "local_pc" }),
  });
  await testPage.goto(`/office/agents/${chiefId}/configuration`);
  await testPage.getByTestId("open-agent-conversation").click();
  await expect(testPage).toHaveURL(/\/workspace\/conversations\/[^/?]+(?:\?.*)?$/);
  const conversationURL = testPage.url();
  const taskId = new URL(conversationURL).pathname.split("/").pop()!;
  const response = await testPage.request.get(`${backend.baseUrl}/api/v1/office/tasks/${taskId}`);
  expect(response.ok()).toBeTruthy();
  const task = await response.json();
  expect(JSON.stringify(task)).toContain(chiefId);
  await testPage.goto(`/office/agents/${chiefId}/configuration`);
  await testPage.getByTestId("open-agent-conversation").click();
  await expect(testPage).toHaveURL(conversationURL);
  await expect(
    testPage.getByText("Conversation with Chief of staff", { exact: true }).first(),
  ).toBeVisible();
  const posted = await testPage.request.post(
    `${backend.baseUrl}/api/v1/office/tasks/${taskId}/comments`,
    {
      data: { body: "Please give me a brief status update." },
    },
  );
  expect(posted.ok()).toBeTruthy();
  await expect
    .poll(
      async () => {
        const result = await testPage.request.get(
          `${backend.baseUrl}/api/v1/office/tasks/${taskId}/comments`,
        );
        const { comments } = await result.json();
        return comments.some(
          (comment: { author_type?: string; authorType?: string }) =>
            (comment.author_type ?? comment.authorType) === "agent",
        );
      },
      { timeout: 30_000 },
    )
    .toBeTruthy();
  const second = await testPage.request.post(
    `${backend.baseUrl}/api/v1/office/tasks/${taskId}/comments`,
    {
      data: { body: "What should happen next?" },
    },
  );
  expect(second.ok()).toBeTruthy();
  await expect
    .poll(
      async () => {
        const result = await testPage.request.get(
          `${backend.baseUrl}/api/v1/office/tasks/${taskId}/comments`,
        );
        const { comments } = await result.json();
        return comments.filter(
          (comment: { author_type?: string; authorType?: string }) =>
            (comment.author_type ?? comment.authorType) === "agent",
        ).length;
      },
      { timeout: 30_000 },
    )
    .toBeGreaterThanOrEqual(2);
  const worker = await officeApi.createAgent(officeSeed.workspaceId, {
    name: "Personal worker",
    role: "worker",
    agent_profile_id: seedData.agentProfileId,
  });
  await officeApi.updateAgent(worker.id as string, {
    executor_preference: JSON.stringify({ type: "local_pc" }),
  });
  const child = await officeApi.createTask(officeSeed.workspaceId, "Delegated status check", {
    parent_id: taskId,
    workflow_id: officeSeed.workflowId,
    description: "Return a concise status result.",
  });
  const childId = child.id as string;
  await officeApi.assignTask(childId, worker.id as string);
  await expect
    .poll(
      async () => {
        const response = await testPage.request.get(
          `${backend.baseUrl}/api/v1/office/tasks/${childId}/comments`,
        );
        const { comments } = await response.json();
        return comments.some((comment: { authorType: string }) => comment.authorType === "agent");
      },
      { timeout: 30_000 },
    )
    .toBeTruthy();
  await officeApi.updateTaskStatus(childId, "done", "Worker result verified.");
  await expect
    .poll(
      async () => {
        const response = await testPage.request.get(
          `${backend.baseUrl}/api/v1/office/tasks/${taskId}/comments`,
        );
        const { comments } = await response.json();
        return comments.filter((comment: { authorType: string }) => comment.authorType === "agent")
          .length;
      },
      { timeout: 30_000 },
    )
    .toBeGreaterThanOrEqual(3);
});

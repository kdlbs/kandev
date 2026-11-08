import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  createGenericWrapper,
  readProfileProbeEvidence,
} from "../../helpers/profile-capability-discovery";
import { checkWrappedPassthrough } from "../../helpers/generic-wrapper-passthrough";

test.use({ trace: "retain-on-failure" });

test("wraps native terminal input, resume, and cancellation", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(180_000);
  await checkWrappedPassthrough(testPage, apiClient, backend, seedData, false);
});

test("wraps ACP chat before and after backend restart", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(180_000);
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("mock-agent is required");
  const evidencePath = path.join(backend.tmpDir, "wrapped-acp.jsonl");
  const profile = await apiClient.createAgentProfile(agent.id, "Wrapped ACP", {
    model: "mock-fast",
    auto_approve: true,
    command_prefix: createGenericWrapper(backend),
    env_vars: [{ key: "MOCK_AGENT_PROFILE_EVIDENCE_FILE", value: evidencePath }],
  });
  let taskId: string | undefined;
  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Wrapped ACP task",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    taskId = task.id;
    if (!task.session_id) throw new Error("session was not created");
    await waitForLatestSessionDone(apiClient, task.id, 1, "initial wrapped ACP turn");
    const receipts = () => readProfileProbeEvidence(evidencePath).filter((item) => item.wrapper);
    expect(receipts()).toHaveLength(1);
    const before = await apiClient.listSessionMessages(task.session_id);
    const assistantCount = before.messages.filter((item) => item.author_type === "agent").length;
    expect(assistantCount).toBeGreaterThan(0);
    await backend.restart();
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.sendMessage("/e2e:simple-message");
    await expect
      .poll(
        async () => {
          const result = await apiClient.listSessionMessages(task.session_id!);
          return result.messages.filter((item) => item.author_type === "agent").length;
        },
        { timeout: 30_000 },
      )
      .toBeGreaterThan(assistantCount);
    expect(receipts()).toHaveLength(2);
  } finally {
    if (taskId) await apiClient.deleteTask(taskId);
    await apiClient.deleteAgentProfile(profile.id, true);
  }
});

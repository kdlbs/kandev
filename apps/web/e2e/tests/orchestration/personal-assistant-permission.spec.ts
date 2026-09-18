import { test, expect } from "../../fixtures/test-base";
import {
  ASSISTANT_ENV,
  seedExampleObjective,
  selectExampleAssistant,
} from "../../helpers/personal-assistant";

// @covers AC-ORCHESTRATION-ASSISTANT-006.1, AC-ORCHESTRATION-ASSISTANT-006.4
test("human permission resolves the native request while the assistant is paused", async ({
  testPage: page,
  backend,
  apiClient: api,
  seedData: seed,
}) => {
  test.setTimeout(120000);
  await backend.restart({ ...ASSISTANT_ENV, AGENTCTL_AUTO_APPROVE_PERMISSIONS: "false" });
  const binding = await selectExampleAssistant(page, backend, api, seed);
  const task = await api.createTask(seed.workspaceId, "Review example workspace access", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  });
  seedExampleObjective(backend, binding, task.id);
  const session = await api.launchSession({
    task_id: task.id,
    agent_profile_id: seed.agentProfileId,
    workflow_step_id: seed.startStepId,
    prompt: "/e2e:kandev-mcp-permission",
  });
  const base = `${backend.baseUrl}/api/v1/orchestration/assistant`;
  let attentionId = "";
  await expect
    .poll(
      async () => {
        const { entries } = await (await page.request.get(`${base}/attention`)).json();
        attentionId =
          entries.find(
            (row: { kind: string; state: string; session_id: string }) =>
              row.kind === "permission" &&
              row.state === "pending" &&
              row.session_id === session.session_id,
          )?.id ?? "";
        return attentionId;
      },
      { timeout: 60000 },
    )
    .not.toBe("");
  await page.reload();
  await page.getByRole("button", { name: "Pause assistant", exact: true }).click();
  await expect(page.getByRole("button", { name: "Resume assistant", exact: true })).toBeVisible();
  const pending = await (await page.request.get(`${base}/attention/${attentionId}/input`)).json();
  expect(pending.input.state).toBe("pending");
  const option = pending.input.permission.options.find(
    (row: { kind: string }) => row.kind === "allow_once",
  );
  expect(option).toBeTruthy();
  await page.getByRole("button", { name: "Review and respond", exact: true }).click();
  await page
    .getByTestId("assistant-attention-card")
    .getByRole("button", { name: option.name, exact: true })
    .click();
  await expect
    .poll(
      async () => {
        const response = await page.request.get(`${base}/attention/${attentionId}/input`);
        return (await response.json()).input.state;
      },
      { timeout: 30000 },
    )
    .toBe("resolved");
  await page.goto(`/tasks/${task.id}?sessionId=${session.session_id}`);
  await expect(page.getByTestId("permission-action-row")).toHaveCount(0);
  await page.goto("/assistant");
  await expect(page.getByRole("button", { name: "Review and respond", exact: true })).toHaveCount(
    0,
  );
  await expect(page.getByRole("button", { name: "Resume assistant", exact: true })).toBeVisible();
  const { objectives } = await (await page.request.get(`${base}/objectives`)).json();
  expect(objectives[0].status).toBe("active");
  expect((await api.listTaskSessions(binding.conversation_id)).sessions).toEqual([]);
});

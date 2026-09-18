import { test, expect } from "../../fixtures/test-base";
import {
  ASSISTANT_ENV,
  seedExampleObjective,
  selectExampleAssistant,
} from "../../helpers/personal-assistant";

// @covers AC-ORCHESTRATION-ASSISTANT-005.1, AC-ORCHESTRATION-ASSISTANT-006.3
test("assistant keeps both worker requests and reconciles a lost answer acknowledgement", async ({
  testPage: page,
  backend,
  apiClient: api,
  seedData: seed,
}) => {
  test.setTimeout(180000);
  page.setDefaultTimeout(15000);
  await backend.restart(ASSISTANT_ENV);
  const binding = await selectExampleAssistant(page, backend, api, seed);
  const task = await api.createTask(seed.workspaceId, "Prepare a sample guide", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  });
  seedExampleObjective(backend, binding, task.id);
  const questions = [
    { id: "heading", prompt: "Which heading should the sample guide use?", answer: "Quick start" },
    {
      id: "format",
      prompt: "Which format should the sample guide use?",
      answer: "Short checklist",
    },
  ];
  const sessions: string[] = [];
  for (const question of questions) {
    const session = await api.launchSession({
      task_id: task.id,
      agent_profile_id: seed.agentProfileId,
      workflow_step_id: seed.startStepId,
      prompt: `e2e:mcp:kandev:ask_user_question_kandev(${JSON.stringify({
        questions: [
          {
            id: question.id,
            prompt: question.prompt,
            options: [
              { label: question.answer, description: "A generic example" },
              { label: "Detailed reference", description: "A longer example" },
            ],
          },
        ],
      })})`,
    });
    sessions.push(session.session_id);
    await expect
      .poll(
        async () =>
          (await api.listSessionMessages(session.session_id)).messages.some(
            (row) => row.type === "clarification_request" && row.metadata?.status === "pending",
          ),
        { timeout: 60000 },
      )
      .toBe(true);
  }
  const base = `${backend.baseUrl}/api/v1/orchestration/assistant`;
  await expect
    .poll(
      async () => {
        const { entries } = await (await page.request.get(`${base}/attention`)).json();
        return entries.filter(
          (row: { kind: string; state: string }) =>
            row.kind === "question" && row.state === "pending",
        ).length;
      },
      { timeout: 60000 },
    )
    .toBe(2);
  let dropped = 0;
  await page.route("**/api/v1/orchestration/assistant/attention/*/resolve", async (route) => {
    if (dropped > 0) return route.continue();
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    dropped++;
    await route.abort("failed");
  });
  for (const [index, question] of questions.entries()) {
    await page.reload();
    const row = page.getByTestId("assistant-attention-row").filter({ hasText: question.prompt });
    await row.getByRole("button", { name: "Review and respond", exact: true }).click();
    await page
      .getByTestId("assistant-attention-card")
      .getByTestId("clarification-option")
      .filter({ hasText: question.answer })
      .click();
    await expect
      .poll(
        async () =>
          (await api.listSessionMessages(sessions[index])).messages.find(
            (entry) => entry.type === "clarification_request",
          )?.metadata?.status,
        { timeout: 30000 },
      )
      .toBe("answered");
  }
  expect(dropped).toBe(1);
  await page.unroute("**/api/v1/orchestration/assistant/attention/*/resolve");
  await backend.restart(ASSISTANT_ENV);
  await page.reload();
  await expect(page.getByRole("button", { name: "Review and respond", exact: true })).toHaveCount(
    0,
  );
  const after = await (await page.request.get(base)).json();
  expect(after.conversation_id).toBe(binding.conversation_id);
  const { objectives } = await (await page.request.get(`${base}/objectives`)).json();
  expect(objectives[0].status).toBe("active");
  for (const session of sessions) {
    expect(
      (await api.listSessionMessages(session)).messages.filter(
        (row) => row.type === "clarification_request",
      ),
    ).toHaveLength(1);
  }
  expect((await api.listTaskSessions(binding.conversation_id)).sessions).toEqual([]);
});

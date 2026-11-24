import { readFile } from "node:fs/promises";
import path from "node:path";

import { expect, test } from "../../fixtures/test-base";
import { openTaskSession, waitForSessionDone } from "../../helpers/session";

type TraceEntry = { event: string; session_id: string; process_id: string };

const HELD = "ACP v2 response before cancellation";
const COMPLETED = "ACP v2 completed response";

test("streams, cancels, and restores an ACP v2 chat", async ({
  backend,
  apiClient,
  seedData,
  testPage,
}) => {
  test.setTimeout(120_000);
  const trace = path.join(backend.tmpDir, "acp-v2-preview.jsonl");
  const release = await backend.useEnv({
    E2E_MOCK_AGENT_ACP_TRACE_FILE: trace,
  });
  try {
    await backend.restart({ KANDEV_FEATURES_ACP_V2: "true" });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "ACP v2 preview chat",
      seedData.agentProfileId,
      {
        description: "/e2e:acp-v2-hold",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const sessionId = task.session_id!;
    expect(sessionId).toBeTruthy();
    const chat = await openTaskSession(testPage, task.id);
    await expect(chat.activeChat().getByText(HELD, { exact: true })).toBeVisible({
      timeout: 30_000,
    });
    const cancel = chat.activeChat().getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible();
    await cancel.click();
    await chat.waitForChatIdle({ timeout: 30_000 });

    await chat.sendMessageViaButton("/e2e:acp-v2-complete");
    await expect(chat.activeChat().getByText(COMPLETED, { exact: true })).toBeVisible();
    await waitForSessionDone(apiClient, task.id, sessionId, "Wait for ACP v2 completion", 30_000);
    const started = await firstTraceEntry(trace, "session_new_v2");
    expect(started.session_id).toBeTruthy();

    await backend.restart({ KANDEV_FEATURES_ACP_V2: "true" });
    const restored = await openTaskSession(testPage, task.id);
    await restored.composerReady();
    await restored.sendMessageViaButton("/e2e:acp-v2-after-restart");
    await expect(restored.activeChat().getByText(COMPLETED, { exact: true })).toHaveCount(2);
    await waitForSessionDone(
      apiClient,
      task.id,
      sessionId,
      "Wait for restored ACP v2 turn",
      30_000,
    );
    const resumed = await firstTraceEntry(trace, "session_resume_v2");
    expect(resumed.session_id).toBe(started.session_id);
    expect(resumed.process_id).not.toBe(started.process_id);
    await expect(restored.activeChat().getByText(HELD, { exact: true })).toHaveCount(1);

    await backend.restart({ KANDEV_FEATURES_ACP_V2: "false" });
    const rolledBack = await openTaskSession(testPage, task.id);
    await rolledBack.composerReady();
    await rolledBack.sendMessageViaButton("/e2e:simple-message");
    await expect(
      rolledBack.activeChat().getByText("This is a simple mock response for e2e testing.", {
        exact: true,
      }),
    ).toBeVisible({ timeout: 30_000 });
    expect((await firstTraceEntry(trace, "session_load")).session_id).toBe(started.session_id);
  } finally {
    const evidence = await readFile(trace).catch(() => undefined);
    if (evidence) {
      await test.info().attach("ACP fixture trace", { body: evidence, contentType: "text/plain" });
    }
    await release();
  }
});

async function firstTraceEntry(path: string, event: string): Promise<TraceEntry> {
  const lines = (await readFile(path, "utf8")).trim().split("\n");
  const match = lines
    .map((line) => JSON.parse(line) as TraceEntry)
    .find((entry) => entry.event === event);
  if (!match) throw new Error(`ACP fixture trace has no ${event} event`);
  return match;
}

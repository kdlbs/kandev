import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, type SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import { createGenericWrapper, readProfileProbeEvidence } from "./profile-capability-discovery";
import { waitForLatestSessionDone } from "./session";
import { SessionPage } from "../pages/session-page";

export async function checkWrappedPassthrough(
  page: Page,
  api: ApiClient,
  backend: BackendContext,
  seed: SeedData,
  touch: boolean,
) {
  const { agents } = await api.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("mock-agent is required");
  const evidencePath = path.join(backend.tmpDir, `passthrough-${touch}.jsonl`);
  const profile = await api.createAgentProfile(agent.id, "Wrapped terminal", {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
    command_prefix: createGenericWrapper(backend),
    env_vars: [{ key: "MOCK_AGENT_PROFILE_EVIDENCE_FILE", value: evidencePath }],
  });
  let taskId: string | undefined;
  try {
    const task = await api.createTaskWithAgent(
      seed.workspaceId,
      "Wrapped terminal task",
      profile.id,
      {
        description: "wrap-first",
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
      },
    );
    taskId = task.id;
    if (!task.session_id) throw new Error("session was not created");
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();
    await expect
      .poll(() => session.readXtermBuffer("passthrough-terminal"), { timeout: 30_000 })
      .toContain("Processed: wrap-first");
    const receipts = () => readProfileProbeEvidence(evidencePath).filter((item) => item.wrapper);
    expect(receipts()).toHaveLength(1);
    await waitForLatestSessionDone(api, task.id, 1, "passthrough idle before restart", 30_000);
    await backend.restart();
    await page.reload();
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded(60_000);
    await session.expectPassthroughHasText("RESUMED", 60_000);
    expect(receipts()).toHaveLength(2);
    const toggle = page.getByTestId("passthrough-toggle-composer");
    if (touch) {
      const bounds = await toggle.boundingBox();
      expect(bounds?.height).toBeGreaterThanOrEqual(44);
      await toggle.tap();
    } else await toggle.click();
    const composer = page.getByTestId("passthrough-composer");
    await composer.locator(".tiptap.ProseMirror").fill("wrap-next");
    const send = composer.getByTestId("submit-message-button");
    if (touch) await send.tap();
    else await send.click();
    await expect
      .poll(() => session.readXtermBuffer("passthrough-terminal"), { timeout: 30_000 })
      .toContain("Processed: wrap-next");
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await api.stopSession({ session_id: task.session_id, force: true });
    await expect
      .poll(
        () =>
          receipts().every(({ launcher_pid: pid }) => {
            if (!pid) return false;
            try {
              process.kill(pid, 0);
              return false;
            } catch {
              return true;
            }
          }),
        { timeout: 20_000 },
      )
      .toBe(true);
  } finally {
    if (taskId) await api.deleteTask(taskId);
    await api.deleteAgentProfile(profile.id, true);
  }
}

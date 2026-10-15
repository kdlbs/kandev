import { expect, type Page, type Locator } from "@playwright/test";
import type { SSHSeedData } from "../fixtures/ssh-test-base";
import type { ApiClient } from "./api-client";
import { execInContainer, readRemoteFile } from "./ssh";
import { SessionPage } from "../pages/session-page";

export type RemoteTUITrace = { kind: string; pid: number; cwd: string; prompt?: string };

export function readPassthroughTrace(seed: SSHSeedData, workspace: string): RemoteTUITrace[] {
  return readRemoteFile(seed.sshTarget, `${workspace}/.kandev/e2e-passthrough.jsonl`)
    .trim()
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line) as RemoteTUITrace);
}

export async function openSSHPassthrough(page: Page, api: ApiClient, seed: SSHSeedData) {
  const { agents } = await api.listAgents();
  const mock = agents.find((agent) => agent.name === "mock-agent");
  if (!mock) throw new Error("SSH passthrough requires the mock agent fixture");
  const profile = await api.createAgentProfile(mock.id, "SSH passthrough fixture", {
    model: "mock-fast",
    cli_passthrough: true,
    cli_flags: [
      { description: "Record remote process identity", flag: "--tui-trace", enabled: true },
    ],
  });
  const initialPrompt = "ssh passthrough initial";
  const task = await api.createTaskWithAgent(seed.workspaceId, "SSH passthrough", profile.id, {
    description: initialPrompt,
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
    executor_profile_id: seed.sshExecutorProfileId,
  });
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForPassthroughLoad();
  await session.waitForPassthroughLoaded();
  await session.expectPassthroughHasText("Mock Agent");
  await session.expectPassthroughHasText(initialPrompt);
  const rows = await api.listSSHSessions(seed.sshExecutorId);
  const row = rows.find((candidate) => candidate.task_id === task.id);
  expect(row?.remote_task_dir).toBeTruthy();
  const workspace = row!.remote_task_dir!;
  await expect
    .poll(
      () =>
        readPassthroughTrace(seed, workspace).filter((entry) =>
          entry.prompt?.includes(initialPrompt),
        ).length,
    )
    .toBe(1);
  const trace = readPassthroughTrace(seed, workspace);
  const started = trace.find((entry) => entry.kind === "start");
  expect(started).toBeTruthy();
  expect(started!.cwd).toBe(workspace);
  expect(trace.filter((entry) => entry.prompt?.includes(initialPrompt))).toHaveLength(1);
  execInContainer(seed.sshTarget, ["kill", "-0", String(started!.pid)]);
  return { task, session, workspace, pid: started!.pid, initialPrompt, profileId: profile.id };
}

export async function typePassthrough(terminal: Locator, text: string) {
  const input = terminal.locator("textarea.xterm-helper-textarea");
  await input.focus();
  await input.pressSequentially(text, { delay: 20 });
  await input.press("Enter");
}

import { test, expect } from "../../fixtures/ssh-test-base";
import {
  openSSHPassthrough,
  readPassthroughTrace,
  typePassthrough,
} from "../../helpers/ssh-passthrough";
import { execInContainer } from "../../helpers/ssh";
import { watchWs } from "../../helpers/causal-waits";
import path from "node:path";
import { captureSSHInterruptionLogs } from "../../helpers/ssh-failure-diagnostics";
import { SessionPage } from "../../pages/session-page";

test("remote passthrough accepts input and keeps its PID after reconnect", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const state = await openSSHPassthrough(testPage, apiClient, seedData);
  await typePassthrough(state.session.passthroughTerminal, "remote follow-up");
  await state.session.expectPassthroughHasText("Processed: remote follow-up");
  expect(
    readPassthroughTrace(seedData, state.workspace).filter(
      (entry) => entry.prompt === "remote follow-up",
    ),
  ).toHaveLength(1);
  await testPage.reload();
  await state.session.waitForPassthroughLoad();
  await state.session.waitForPassthroughLoaded();
  await state.session.expectPassthroughHasText("remote follow-up");
  execInContainer(seedData.sshTarget, ["kill", "-0", String(state.pid)]);
  expect(
    readPassthroughTrace(seedData, state.workspace).filter((entry) => entry.kind === "start"),
  ).toHaveLength(1);
});

const interruptionTest =
  "remote passthrough survives backend interruption without another initial prompt";
let interruptionLog: string | undefined;

test.beforeEach(() => {
  interruptionLog = undefined;
});

test.afterEach(async ({ backend }, testInfo) => {
  if (testInfo.title === interruptionTest) {
    await captureSSHInterruptionLogs(testInfo, backend, interruptionLog);
  }
});

test(interruptionTest, async ({ testPage, apiClient, seedData, backend }) => {
  const state = await openSSHPassthrough(testPage, apiClient, seedData);
  const taskDirectory = path.basename(state.workspace);
  const sessionID = state.task.session_id;
  if (/^task-[a-f0-9-]{36}$/.test(taskDirectory) && /^[a-f0-9-]{36}$/.test(sessionID ?? "")) {
    interruptionLog = path.join(
      seedData.sshTarget.workDir,
      "remote-home/tasks",
      taskDirectory,
      ".kandev/sessions",
      sessionID!,
      "agentctl.log",
    );
  }
  const backendPID = backend.pid();
  expect(backendPID).toBeTruthy();
  process.kill(-backendPID!, "SIGKILL");
  execInContainer(seedData.sshTarget, ["kill", "-0", String(state.pid)]);
  await backend.restart();
  await testPage.reload();
  await state.session.waitForPassthroughLoad();
  await state.session.waitForPassthroughLoaded();
  await state.session.expectPassthroughHasText("Mock Agent");
  await typePassthrough(state.session.passthroughTerminal, "after backend restart");
  await state.session.expectPassthroughHasText("Processed: after backend restart");
  const trace = readPassthroughTrace(seedData, state.workspace);
  expect(trace.filter((entry) => entry.kind === "start")).toHaveLength(1);
  expect(trace.filter((entry) => entry.prompt?.includes(state.initialPrompt))).toHaveLength(1);
});

test("remote MCP uses the active task and session", async ({ testPage, apiClient, seedData }) => {
  const state = await openSSHPassthrough(testPage, apiClient, seedData);
  await typePassthrough(state.session.passthroughTerminal, "/e2e:mcp-context");
  await state.session.expectPassthroughHasText("Remote MCP:");
  await state.session.expectPassthroughHasText(state.task.session_id!);
  await state.session.expectPassthroughHasText('"is_current": true');
});

test("Stop ends the remote CLI and Resume preserves its workspace", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const ws = watchWs(testPage);
  const state = await openSSHPassthrough(testPage, apiClient, seedData);
  expect(
    (await apiClient.stopSession({ session_id: state.task.session_id!, force: true })).success,
  ).toBe(true);
  await expect
    .poll(() =>
      execInContainer(seedData.sshTarget, [
        "sh",
        "-c",
        `if kill -0 ${state.pid} 2>/dev/null; then echo alive; else echo stopped; fi`,
      ]).trim(),
    )
    .toBe("stopped");
  execInContainer(seedData.sshTarget, ["test", "-d", state.workspace]);
  await expect(state.session.recoveryResumeButton()).toBeVisible();
  const resumed = ws.waitForResponse("session.recover", { timeout: 60_000 });
  await state.session.recoveryResumeButton().click();
  await resumed;
  await state.session.waitForPassthroughLoaded();
  await state.session.expectPassthroughHasText("Mock Agent (RESUMED)");
  await typePassthrough(state.session.passthroughTerminal, "after explicit resume");
  await state.session.expectPassthroughHasText("Processed: after explicit resume");
  const trace = readPassthroughTrace(seedData, state.workspace);
  const starts = trace.filter((entry) => entry.kind === "start");
  expect(starts).toHaveLength(2);
  expect(starts[1].pid).not.toBe(state.pid);
  expect(starts[1].cwd).toBe(state.workspace);
  expect(trace.filter((entry) => entry.prompt?.includes(state.initialPrompt))).toHaveLength(1);
});

test("sibling passthrough sessions keep separate remote input", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const state = await openSSHPassthrough(testPage, apiClient, seedData);
  const sibling = await apiClient.launchSession({
    task_id: state.task.id,
    intent: "start",
    agent_profile_id: state.profileId,
    executor_profile_id: seedData.sshExecutorProfileId,
    workflow_step_id: seedData.startStepId,
    prompt: "sibling initial",
  });
  expect(sibling.session_id).not.toBe(state.task.session_id);
  await testPage.goto(`/t/${state.task.id}?sessionId=${sibling.session_id}`);
  const session = new SessionPage(testPage);
  await session.sessionTabBySessionId(sibling.session_id).click();
  await session.waitForPassthroughLoad();
  await session.waitForPassthroughLoaded();
  await session.expectPassthroughHasText("Mock Agent");
  await typePassthrough(session.passthroughTerminal, "sibling-only input");
  await session.expectPassthroughHasText("Processed: sibling-only input");
  const trace = readPassthroughTrace(seedData, state.workspace);
  const accepted = trace.filter((entry) => entry.prompt === "sibling-only input");
  expect(accepted).toHaveLength(1);
  expect(accepted[0].pid).not.toBe(state.pid);
  execInContainer(seedData.sshTarget, ["kill", "-0", String(state.pid)]);
  await testPage.goto(`/t/${state.task.id}?sessionId=${state.task.session_id}`);
  await session.sessionTabBySessionId(state.task.session_id!).click();
  await session.waitForPassthroughLoad();
  await session.waitForPassthroughLoaded();
  await typePassthrough(session.passthroughTerminal, "original-only input");
  await session.expectPassthroughHasText("Processed: original-only input");
  const originalInput = readPassthroughTrace(seedData, state.workspace).filter(
    (entry) => entry.prompt === "original-only input",
  );
  expect(originalInput).toHaveLength(1);
  expect(originalInput[0].pid).toBe(state.pid);
});

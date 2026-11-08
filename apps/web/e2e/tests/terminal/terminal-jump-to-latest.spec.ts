import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { writeFile, rm } from "node:fs/promises";
import { join } from "node:path";
import {
  INITIAL_MARKER,
  LATER_MARKER,
  OUTPUT_COMMAND,
  terminalState,
  verifyJump,
} from "./terminal-jump-helpers";
import { attachShellInputCapture } from "../../helpers/ws-capture";
import { waitForHttp } from "../../helpers/causal-waits";

test("task shell returns to latest output", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const { frames } = attachShellInputCapture(testPage);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Jump shell",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await session.clickTab("Terminal");
  await session.typeInTerminal(OUTPUT_COMMAND);
  await verifyJump(
    testPage,
    testPage.getByTestId("terminal-panel").filter({ visible: true }).last(),
    false,
    () => prCapture.screenshot("desktop", { caption: "Task terminal jump to latest output" }),
    frames,
  );
});

test("dev-server output returns to latest output", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(120_000);
  const gate = join(backend.tmpDir, `jump-to-latest-gate-${testInfo.workerIndex}`);
  const initialGate = join(backend.tmpDir, `jump-to-latest-initial-${testInfo.workerIndex}`);
  const quotedGate = `'${gate.replaceAll("'", "'\\''")}'`;
  const quotedInitialGate = `'${initialGate.replaceAll("'", "'\\''")}'`;
  const command = `while [ ! -f ${quotedInitialGate} ]; do sleep 0.1; done; seq 1 200 | sed 's/^/jump-line-/'; while [ ! -f ${quotedGate} ]; do sleep 0.1; done; printf 'jump-%s-%s\\n' later marker; sleep 60`;
  const processFrames: Array<{ process_id?: string; session_id?: string; data?: string }> = [];
  const { frames: inputFrames } = attachShellInputCapture(testPage);
  testPage.on("websocket", (socket) => {
    socket.on("framereceived", ({ payload }) => {
      if (typeof payload !== "string" || !payload.includes("session.process.output")) return;
      const message = JSON.parse(payload) as {
        action?: string;
        payload?: (typeof processFrames)[number];
      };
      if (message.action === "session.process.output" && message.payload)
        processFrames.push(message.payload);
    });
  });
  const original = (await apiClient.getRepository(seedData.repositoryId)).dev_script ?? "";
  let childProcess: { id: string; session_id: string } | undefined;
  const evidence: Record<string, unknown> = {};
  try {
    await apiClient.updateRepository(seedData.repositoryId, { dev_script: command });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Jump dev server",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    const started = waitForHttp(testPage, "POST", /\/processes\/start$/);
    const initialLoad = waitForHttp(testPage, "GET", /\/processes\/[^/]+$/, {
      predicate: (response) =>
        new URL(response.url()).searchParams.get("include_output") === "true",
    });
    await testPage.getByTestId("dev-server-preview-toggle").click();
    const response = await started;
    ({ process: childProcess } = (await response.json()) as {
      process: { id: string; session_id: string };
    });
    expect(childProcess.session_id).toBe(task.session_id);
    const loaded = await initialLoad;
    expect(loaded.ok()).toBe(true);
    expect(new URL(loaded.url()).pathname.endsWith(`/processes/${childProcess.id}`)).toBe(true);
    const panel = testPage.getByTestId("dev-server-panel").filter({ visible: true });
    await expect(panel).toBeVisible();
    const readStore = () =>
      testPage.evaluate(
        ({ sessionId, processId }) => {
          type State = {
            tasks: { activeSessionId: string | null };
            processes: {
              devProcessBySessionId: Record<string, string>;
              outputsByProcessId: Record<string, string>;
            };
          };
          const store = (window as Window & { __KANDEV_E2E_STORE__?: { getState: () => State } })
            .__KANDEV_E2E_STORE__;
          if (!store) throw new Error("E2E store unavailable");
          const state = store.getState();
          return {
            activeSession: state.tasks.activeSessionId,
            selectedProcess: state.processes.devProcessBySessionId[sessionId],
            output: state.processes.outputsByProcessId[processId] ?? "",
          };
        },
        { sessionId: childProcess!.session_id, processId: childProcess!.id },
      );
    await expect.poll(async () => (await readStore()).selectedProcess).toBe(childProcess.id);
    await writeFile(initialGate, "release");
    await expect.poll(async () => (await readStore()).output).toContain(INITIAL_MARKER);
    await expect.poll(async () => (await terminalState(panel)).text).toContain(INITIAL_MARKER);
    const initial = await terminalState(panel);
    const action = panel.getByRole("button", { name: "Jump to latest output" });
    await expect(action).toHaveCount(0);
    await panel.locator(".xterm").hover();
    await testPage.mouse.wheel(0, -450);
    await expect
      .poll(async () => (await terminalState(panel)).viewport)
      .toBeLessThan(initial.viewport);
    await expect(action).toBeVisible();
    const before = await terminalState(panel);
    const storeBefore = await readStore();
    expect(storeBefore.activeSession).toBe(childProcess.session_id);
    expect(storeBefore.output).not.toContain(LATER_MARKER);
    expect(before.text).not.toContain(LATER_MARKER);
    evidence.before = { viewport: before.viewport, selectedProcess: storeBefore.selectedProcess };
    await writeFile(gate, "release");
    await expect
      .poll(() =>
        processFrames.some(
          (frame) =>
            frame.process_id === childProcess!.id &&
            frame.session_id === childProcess!.session_id &&
            frame.data?.includes(LATER_MARKER),
        ),
      )
      .toBe(true);
    await expect.poll(async () => (await readStore()).output).toContain(LATER_MARKER);
    await expect.poll(async () => (await terminalState(panel)).text).toContain(LATER_MARKER);
    const after = await terminalState(panel);
    expect(after.viewport).toBe(before.viewport);
    const inputCount = inputFrames.length;
    await action.click();
    await expect(action).toHaveCount(0);
    await expect
      .poll(async () => (await terminalState(panel)).viewport)
      .toBeGreaterThan(before.viewport);
    expect(inputFrames.length).toBe(inputCount);
    evidence.after = {
      processId: childProcess.id,
      sessionId: childProcess.session_id,
      viewport: after.viewport,
      jumpedViewport: (await terminalState(panel)).viewport,
      inputFramesBefore: inputCount,
      inputFramesAfter: inputFrames.length,
      matchingFrame: processFrames.find(
        (frame) =>
          frame.process_id === childProcess!.id &&
          frame.session_id === childProcess!.session_id &&
          frame.data?.includes(LATER_MARKER),
      ),
    };
  } finally {
    try {
      const evidencePath = process.env.KANDEV_RUN_QUIET_DIR
        ? join(process.env.KANDEV_RUN_QUIET_DIR, "dev-acceptance-evidence.json")
        : testInfo.outputPath("dev-acceptance-evidence.json");
      await writeFile(evidencePath, JSON.stringify(evidence, null, 2));
    } finally {
      try {
        if (childProcess)
          await apiClient.rawRequest(
            "POST",
            `/api/v1/task-sessions/${childProcess.session_id}/processes/${childProcess.id}/stop`,
          );
      } finally {
        try {
          await apiClient.updateRepository(seedData.repositoryId, { dev_script: original });
        } finally {
          await rm(gate, { force: true });
          await rm(initialGate, { force: true });
        }
      }
    }
  }
});

import type { Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { expect, type SeedData } from "../fixtures/test-base";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { waitForSessionState } from "./session";

type ShellScenario = {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  touch: boolean;
  exitCode: number | null;
  cancel?: boolean;
};

export async function exerciseIncrementalShellOutput({
  testPage,
  apiClient,
  seedData,
  backend,
  touch,
  exitCode,
  cancel = false,
}: ShellScenario) {
  const id = randomUUID();
  const liveGate = path.join(backend.tmpDir, `${id}-live`);
  const finalGate = path.join(backend.tmpDir, `${id}-final`);
  const directive = (value: Record<string, unknown>) =>
    `e2e:shell_stream(${JSON.stringify({ id, ...value })})`;
  const script = [
    directive({ action: "start", command: "controlled-output", seed_bytes: 262144 }),
    `e2e:await_file(${JSON.stringify(liveGate)})`,
    directive({ action: "delta", delta: "\nAFTER_SATURATION\n" }),
    `e2e:await_file(${JSON.stringify(finalGate)})`,
    directive({
      action: "finish",
      stdout: "AUTHORITATIVE_FINAL",
      stderr: "FINAL_STDERR",
      exit_code: exitCode,
    }),
    'e2e:message("stream finished")',
  ].join("\n");
  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Incremental shell ${id}`,
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const openOutput = async () => {
      const disclosure = session.activeChat().getByRole("button", { name: "Show command output" });
      await expect(disclosure).toHaveCount(1);
      await expect(disclosure).toBeVisible();
      if (touch) await disclosure.tap();
      else await disclosure.click();
    };
    await openOutput();
    const output = session.activeChat().getByTestId("tool-execute-output");
    await expect(output).toHaveCount(1);
    await expect(output).toContainText("xxxxxxxxxxxxxxxx");
    fs.writeFileSync(liveGate, "release");
    await expect(output).toContainText("AFTER_SATURATION");
    await testPage.reload();
    await session.waitForLoad();
    await openOutput();
    await expect(output).toContainText("AFTER_SATURATION");
    expect(compactEventCount(backend.tmpDir, id)).toBeGreaterThan(0);
    if (cancel) {
      const { sessions } = await apiClient.listTaskSessions(task.id);
      expect(sessions).toHaveLength(1);
      const pause = session.cancelAgentButton();
      await expect(pause).toHaveCount(1);
      if (touch) await pause.tap();
      else await pause.click();
      await waitForSessionState(apiClient, {
        taskId: task.id,
        sessionId: sessions[0].id,
        expectedState: "WAITING_FOR_INPUT",
        message: "cancelled compact-output turn should settle",
      });
      await expect(output).toContainText("AFTER_SATURATION");
      await expect(session.activeChat()).toContainText("Exit code unavailable");
      await testPage.reload();
      await session.waitForLoad();
      await openOutput();
      await expect(output).toContainText("AFTER_SATURATION");
      await expect(session.activeChat()).toContainText("Exit code unavailable");
      return;
    }
    fs.writeFileSync(finalGate, "release");
    await expect(output).toContainText("AUTHORITATIVE_FINAL");
    await expect(output).toContainText("FINAL_STDERR");
    await expect(output).not.toContainText("AFTER_SATURATION");
    await expect(session.activeChat()).toContainText(
      exitCode === null ? "Exit code unavailable" : `Exit code ${exitCode}`,
    );
    await testPage.reload();
    await session.waitForLoad();
    await openOutput();
    await expect(output).toContainText("AUTHORITATIVE_FINAL");
    await expect(output).toContainText("FINAL_STDERR");
    await expect(output).not.toContainText("AFTER_SATURATION");
    await expect(session.activeChat()).toContainText(
      exitCode === null ? "Exit code unavailable" : `Exit code ${exitCode}`,
    );
  } finally {
    // Release owned gates even when an assertion fails so no prompt stays blocked.
    fs.writeFileSync(liveGate, "release");
    fs.writeFileSync(finalGate, "release");
  }
}

function compactEventCount(tmpDir: string, toolId: string): number {
  if (!path.basename(tmpDir).startsWith("kandev-e2e-"))
    throw new Error("Expected owned E2E fixture");
  const script = [
    "import sqlite3,sys",
    "db=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True)",
    "print(db.execute(\"SELECT COUNT(*) FROM agent_delivery_inbox WHERE event_type='shell_output_update' AND json_extract(payload,'$.tool_call_id')=?\",(sys.argv[2],)).fetchone()[0])",
  ].join("\n");
  return Number(
    execFileSync("python3", ["-c", script, path.join(tmpDir, "kandev.db"), toolId], {
      encoding: "utf8",
    }).trim(),
  );
}

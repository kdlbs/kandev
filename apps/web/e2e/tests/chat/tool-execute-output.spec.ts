import { test, expect, type SeedData } from "../../fixtures/test-base";
import { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

type ShellOutput = {
  exit_code?: number;
  stdout?: string;
  stderr?: string;
};

type SeedShellMessageOptions = {
  title: string;
  command: string;
  status: "complete" | "error";
  output: ShellOutput;
};

async function seedShellMessage(
  apiClient: ApiClient,
  seedData: SeedData,
  options: SeedShellMessageOptions,
) {
  const task = await apiClient.createTask(seedData.workspaceId, options.title, {
    description: "seeded shell command output",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    agentProfileId: seedData.agentProfileId,
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "tool_execute",
    content: options.command,
    metadata: {
      status: options.status,
      tool_call_id: `tool-${options.title}`,
      normalized: {
        shell_exec: { command: options.command, work_dir: "/workspace", output: options.output },
      },
    },
  });
  return task.id;
}

test.describe("shell command output", () => {
  test("shows persisted success, failure, and unknown exit results", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const scenarios = [
      {
        title: "Successful Command",
        command: "printf successful-command",
        status: "complete" as const,
        output: { exit_code: 0, stdout: "successful output\n" },
        statusLabel: "Command succeeded",
        exitLabel: "Exit code 0",
        outputText: "successful output",
      },
      {
        title: "Failed Command",
        command: "printf failed-command",
        status: "error" as const,
        output: { exit_code: 7, stderr: "failed output\n" },
        statusLabel: "Command failed",
        exitLabel: "Exit code 7",
        outputText: "failed output",
      },
      {
        title: "Unknown Exit Command",
        command: "printf unknown-exit-command",
        status: "complete" as const,
        output: { stdout: "unknown exit output\n" },
        statusLabel: null,
        exitLabel: "Exit code unavailable",
        outputText: "unknown exit output",
      },
    ];

    for (const scenario of scenarios) {
      const taskId = await seedShellMessage(apiClient, seedData, {
        title: scenario.title,
        command: scenario.command,
        status: scenario.status,
        output: scenario.output,
      });
      await testPage.goto(`/t/${taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const chat = session.activeChat();
      const command = chat.getByTestId("tool-execute-command").filter({
        hasText: scenario.command,
      });
      await expect(command).toBeVisible({ timeout: 15_000 });
      if (scenario.statusLabel) {
        await expect(chat.getByLabel(scenario.statusLabel)).toBeVisible();
      } else {
        await expect(chat.getByLabel("Command succeeded")).toHaveCount(0);
        await expect(chat.getByLabel("Command failed")).toHaveCount(0);
      }

      await command.click();
      await expect(chat.getByText(scenario.outputText)).toBeVisible();
      await expect(chat.getByText(scenario.exitLabel)).toBeVisible();
    }
  });
});

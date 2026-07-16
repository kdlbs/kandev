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
  status: "running" | "complete" | "error";
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
  test("keeps full commands visible and fetches completed output only after expansion", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const command = `printf '%s\\n' ${"complete-command-segment-".repeat(48)}`;
    const transcript = "persisted transcript loaded on demand";
    const taskId = await seedShellMessage(apiClient, seedData, {
      title: "Lazy Completed Command",
      command,
      status: "error",
      output: { exit_code: 7, stdout: `${transcript}\n` },
    });
    const shellOutputRequests: string[] = [];
    testPage.on("request", (request) => {
      if (request.url().endsWith("/shell-output")) shellOutputRequests.push(request.url());
    });

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const chat = session.activeChat();
    const commandRow = chat.getByTestId("tool-execute-command").filter({ hasText: command });
    const disclosure = chat.getByRole("button", { name: "Show command output" });

    await expect(commandRow).toBeVisible({ timeout: 15_000 });
    await expect(commandRow).toHaveText(command);
    await expect(disclosure).toBeVisible();
    await expect(chat.getByTestId("tool-execute-output")).toHaveCount(0);
    await expect(chat.getByText(transcript)).toHaveCount(0);
    expect(shellOutputRequests).toHaveLength(0);

    const wrapsAcrossLines = await commandRow.evaluate((element) => {
      const lineHeight = Number.parseFloat(getComputedStyle(element).lineHeight);
      return element.getBoundingClientRect().height > lineHeight * 2;
    });
    expect(wrapsAcrossLines).toBe(true);

    const responsePromise = testPage.waitForResponse(
      (response) => response.url().endsWith("/shell-output") && response.status() === 200,
    );
    await disclosure.click();
    await responsePromise;

    await expect(chat.getByText(transcript)).toBeVisible();
    await expect(chat.getByText("Exit code 7")).toBeVisible();
    await expect(chat.getByLabel("Command failed")).toBeVisible();
    expect(shellOutputRequests).toHaveLength(1);
  });

  test("refreshes expanded running output and stops after the terminal snapshot", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const command = "printf controlled-running-command";
    const taskId = await seedShellMessage(apiClient, seedData, {
      title: "Controlled Running Command",
      command,
      status: "running",
      output: { stdout: "persisted output is replaced by routed snapshots\n" },
    });
    const snapshots = [
      {
        message_id: "controlled-running-message",
        status: "running",
        updated_at: "2026-07-16T12:00:00Z",
        output: { stdout: "partial running transcript\n" },
      },
      {
        message_id: "controlled-running-message",
        status: "complete",
        updated_at: "2026-07-16T12:00:01Z",
        output: { exit_code: 0, stdout: "final running transcript\n" },
      },
    ];
    let requestCount = 0;
    await testPage.route("**/api/v1/task-sessions/*/messages/*/shell-output", async (route) => {
      const snapshot = snapshots[Math.min(requestCount, snapshots.length - 1)];
      requestCount += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(snapshot),
      });
    });

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const chat = session.activeChat();
    const disclosure = chat.getByRole("button", { name: "Show command output" });

    await expect(chat.getByTestId("tool-execute-command").filter({ hasText: command })).toBeVisible(
      {
        timeout: 15_000,
      },
    );
    expect(requestCount).toBe(0);
    await disclosure.click();

    await expect(chat.getByText("partial running transcript")).toBeVisible();
    await expect.poll(() => requestCount, { timeout: 4_000 }).toBe(2);
    await expect(chat.getByText("final running transcript")).toBeVisible();
    await expect(chat.getByText("Exit code 0")).toBeVisible();
    await testPage.waitForTimeout(1_250);
    expect(requestCount).toBe(2);
  });

  test("preserves successful and unknown completed result semantics", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const scenarios = [
      {
        title: "Successful Command",
        command: "printf successful-command",
        output: { exit_code: 0, stdout: "successful output\n" },
        statusLabel: "Command succeeded",
        exitLabel: "Exit code 0",
        outputText: "successful output",
      },
      {
        title: "Unknown Exit Command",
        command: "printf unknown-exit-command",
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
        status: "complete",
        output: scenario.output,
      });
      await testPage.goto(`/t/${taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const chat = session.activeChat();
      const commandRow = chat.getByTestId("tool-execute-command").filter({
        hasText: scenario.command,
      });
      await expect(commandRow).toBeVisible({ timeout: 15_000 });
      await expect(chat.getByText(scenario.outputText)).toHaveCount(0);

      const responsePromise = testPage.waitForResponse(
        (response) => response.url().endsWith("/shell-output") && response.status() === 200,
      );
      await chat.getByRole("button", { name: "Show command output" }).click();
      await responsePromise;

      await expect(chat.getByText(scenario.outputText)).toBeVisible();
      await expect(chat.getByText(scenario.exitLabel)).toBeVisible();
      if (scenario.statusLabel) {
        await expect(chat.getByLabel(scenario.statusLabel)).toBeVisible();
      } else {
        await expect(chat.getByLabel("Command succeeded")).toHaveCount(0);
        await expect(chat.getByLabel("Command failed")).toHaveCount(0);
      }
    }
  });
});
